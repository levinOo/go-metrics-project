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

func (m *MetricsStruct) UpdateMetrics(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	if len(req.Metrics) == 0 {
		m.logger.Info("Received empty metrics list")
		return &pb.UpdateMetricsResponse{}, nil
	}

	for _, metric := range req.Metrics {
		if err := m.saveMetric(metric); err != nil {
			m.logger.Errorf("Failed to save metric %s: %v", metric.Id, err)
			return nil, err
		}
	}

	m.logger.Infof("Successfully updated %d metrics", len(req.Metrics))
	return &pb.UpdateMetricsResponse{}, nil
}

func (m *MetricsStruct) saveMetric(metric *pb.Metric) error {
	switch metric.Type {
	case pb.Metric_GAUGE:
		return m.store.SetGauge(metric.Id, repository.Gauge(metric.Value))
	case pb.Metric_COUNTER:
		return m.store.SetCounter(metric.Id, repository.Counter(metric.Delta))
	default:
		return fmt.Errorf("unknown metric type: %v", metric.Type)
	}
}

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

func StartGRPCServer(addr string, store repository.Storage, sugar *zap.SugaredLogger, trustedSubnet string) (*grpc.Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		sugar.Errorf("Failed to create listener: %v", err)
		return nil, err
	}

	s := grpc.NewServer(grpc.UnaryInterceptor(trustedIPInterceptor(trustedSubnet, sugar)))

	pb.RegisterMetricsServer(s, &MetricsStruct{
		store:         store,
		logger:        sugar,
		trustedSubnet: trustedSubnet,
	})

	sugar.Infof("gRPC server started on %s", addr)

	go func() {
		if err := s.Serve(listener); err != nil {
			sugar.Errorf("gRPC server error: %v", err)
		}
	}()

	return s, nil
}
