package grpc

import (
	"context"
	"fmt"
	"net"
	"strings"

	pb "github.com/levinOo/go-metrics-project/internal/proto"
	"github.com/levinOo/go-metrics-project/internal/repository"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type MetricsStruct struct {
	pb.UnimplementedMetricsServer
	store         repository.Storage
	logger        *zap.SugaredLogger
	trustedSubnet string
}

// UpdateMetrics обрабатывает запрос на обновление метрик.
// Используем Opaque API - только геттеры для чтения полей.
func (m *MetricsStruct) UpdateMetrics(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	// Используем геттер для получения метрик из запроса
	metrics := req.GetMetrics()

	if len(metrics) == 0 {
		m.logger.Info("Received empty metrics list")
		return &pb.UpdateMetricsResponse{}, nil
	}

	for _, metric := range metrics {
		if err := m.saveMetric(metric); err != nil {
			// Используем геттер для получения ID метрики
			m.logger.Errorf("Failed to save metric %s: %v", metric.GetId(), err)
			return nil, err
		}
	}

	m.logger.Infof("Successfully updated %d metrics", len(metrics))
	return &pb.UpdateMetricsResponse{}, nil
}

// saveMetric сохраняет одну метрику в хранилище.
// Используем Opaque API - только геттеры для чтения полей.
func (m *MetricsStruct) saveMetric(metric *pb.Metric) error {
	// Используем геттеры для получения значений всех полей
	metricType := metric.GetType()
	metricId := metric.GetId()

	switch metricType {
	case pb.Metric_GAUGE:
		metricValue := metric.GetValue()
		return m.store.SetGauge(metricId, repository.Gauge(metricValue))
	case pb.Metric_COUNTER:
		metricDelta := metric.GetDelta()
		return m.store.SetCounter(metricId, repository.Counter(metricDelta))
	default:
		return fmt.Errorf("unknown metric type: %v", metricType)
	}
}

// trustedIPInterceptor создает gRPC interceptor для проверки доверенных IP адресов.
func trustedIPInterceptor(trustedSubnet string, sugar *zap.SugaredLogger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Если доверенная подсеть не указана, пропускаем проверку
		if trustedSubnet == "" {
			return handler(ctx, req)
		}

		// Получаем metadata из контекста
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			sugar.Error("Failed to get metadata from context")
			return nil, fmt.Errorf("failed to get metadata from context")
		}

		// Извлекаем IP из метаданных с ключом x-real-ip
		realIP := md.Get("x-real-ip")
		if len(realIP) == 0 {
			sugar.Error("x-real-ip metadata is missing")
			return nil, fmt.Errorf("x-real-ip metadata is missing")
		}

		clientIP := realIP[0]
		ip := net.ParseIP(clientIP)
		if ip == nil {
			sugar.Errorf("Invalid client IP in metadata: %s", clientIP)
			return nil, fmt.Errorf("invalid client IP address")
		}

		// Проверяем, является ли это CIDR или одиночным IP
		if strings.Contains(trustedSubnet, "/") {
			// CIDR нотация
			_, trustedNet, err := net.ParseCIDR(trustedSubnet)
			if err != nil {
				sugar.Errorf("Invalid trusted subnet format %s: %v", trustedSubnet, err)
				return nil, fmt.Errorf("invalid trusted subnet configuration")
			}

			if !trustedNet.Contains(ip) {
				sugar.Warnf("IP %s is not in trusted subnet %s", clientIP, trustedSubnet)
				return nil, fmt.Errorf("IP %s is not in trusted subnet", clientIP)
			}
		} else {
			// Одиночный IP адрес
			trustedIP := net.ParseIP(trustedSubnet)
			if trustedIP == nil {
				sugar.Errorf("Invalid trusted IP format %s", trustedSubnet)
				return nil, fmt.Errorf("invalid trusted IP configuration")
			}

			if !ip.Equal(trustedIP) {
				sugar.Warnf("IP %s does not match trusted IP %s", clientIP, trustedSubnet)
				return nil, fmt.Errorf("IP %s does not match trusted IP", clientIP)
			}
		}

		sugar.Infof("Request from trusted IP: %s", clientIP)
		return handler(ctx, req)
	}
}

// StartGRPCServer создает и запускает gRPC сервер.
func StartGRPCServer(addr string, store repository.Storage, sugar *zap.SugaredLogger, trustedSubnet string) (*grpc.Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		sugar.Errorf("Failed to create listener: %v", err)
		return nil, err
	}

	// Создаем gRPC сервер с interceptor для проверки IP
	s := grpc.NewServer(grpc.UnaryInterceptor(trustedIPInterceptor(trustedSubnet, sugar)))

	// Регистрируем сервис метрик
	pb.RegisterMetricsServer(s, &MetricsStruct{
		store:         store,
		logger:        sugar,
		trustedSubnet: trustedSubnet,
	})

	sugar.Infof("gRPC server started on %s", addr)

	// Запускаем сервер в отдельной горутине
	go func() {
		if err := s.Serve(listener); err != nil {
			sugar.Errorf("gRPC server error: %v", err)
		}
	}()

	return s, nil
}
