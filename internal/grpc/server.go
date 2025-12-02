// Package grpc предоставляет gRPC-сервер для приема метрик от агентов.
package grpc

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/levinOo/go-metrics-project/internal/proto"
	"github.com/levinOo/go-metrics-project/internal/repository"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MetricsServer реализует gRPC-сервис для обновления метрик.
type MetricsServer struct {
	proto.UnimplementedMetricsServer
	store         repository.Storage
	logger        *zap.SugaredLogger
	trustedSubnet string
}

// NewMetricsServer создает новый экземпляр gRPC-сервера для метрик.
func NewMetricsServer(store repository.Storage, logger *zap.SugaredLogger, trustedSubnet string) *MetricsServer {
	return &MetricsServer{
		store:         store,
		logger:        logger,
		trustedSubnet: trustedSubnet,
	}
}

// UpdateMetrics реализует RPC-метод для обновления метрик.
// Принимает батч метрик и сохраняет их в хранилище.
func (s *MetricsServer) UpdateMetrics(ctx context.Context, req *proto.UpdateMetricsRequest) (*proto.UpdateMetricsResponse, error) {
	// Проверка IP-адреса агента из метаданных
	if s.trustedSubnet != "" {
		if err := s.checkIPTrustedSubnet(ctx); err != nil {
			s.logger.Warnw("IP verification failed", "error", err)
			return nil, status.Error(codes.PermissionDenied, "IP address not in trusted subnet")
		}
	}

	if len(req.Metrics) == 0 {
		return &proto.UpdateMetricsResponse{}, nil
	}

	// Преобразование proto метрик во внутренние модели и сохранение
	for _, metric := range req.Metrics {
		if err := s.saveMetric(metric); err != nil {
			s.logger.Errorw("Failed to save metric", "id", metric.Id, "error", err)
			return nil, status.Errorf(codes.Internal, "failed to save metric: %v", err)
		}
	}

	s.logger.Infow("Metrics updated successfully", "count", len(req.Metrics))
	return &proto.UpdateMetricsResponse{}, nil
}

// saveMetric сохраняет одиночную метрику в хранилище.
func (s *MetricsServer) saveMetric(m *proto.Metric) error {
	switch m.Type {
	case proto.Metric_GAUGE:
		return s.store.SetGauge(m.Id, repository.Gauge(m.Value))
	case proto.Metric_COUNTER:
		return s.store.SetCounter(m.Id, repository.Counter(m.Delta))
	default:
		return fmt.Errorf("unknown metric type: %v", m.Type)
	}
}

// checkIPTrustedSubnet проверяет, принадлежит ли IP-адрес агента доверенной подсети.
func (s *MetricsServer) checkIPTrustedSubnet(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("missing metadata")
	}

	values := md.Get("x-real-ip")
	if len(values) == 0 {
		return fmt.Errorf("missing x-real-ip header")
	}

	agentIP := values[0]
	return isIPInSubnet(agentIP, s.trustedSubnet)
}

// isIPInSubnet проверяет, принадлежит ли IP-адрес указанной подсети.
func isIPInSubnet(ipStr string, subnetStr string) error {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipStr)
	}

	// Если указан одиночный IP, проверяем точное совпадение
	if !strings.Contains(subnetStr, "/") {
		if ip.String() == subnetStr {
			return nil
		}
		return fmt.Errorf("IP %s not in trusted IPs", ipStr)
	}

	// Парсим CIDR подсеть
	_, ipnet, err := net.ParseCIDR(subnetStr)
	if err != nil {
		return fmt.Errorf("invalid CIDR subnet: %s, error: %w", subnetStr, err)
	}

	if ipnet.Contains(ip) {
		return nil
	}

	return fmt.Errorf("IP %s not in trusted subnet %s", ipStr, subnetStr)
}

// StartGRPCServer запускает gRPC-сервер на указанном адресе.
func StartGRPCServer(addr string, store repository.Storage, logger *zap.SugaredLogger, trustedSubnet string) (*grpc.Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	server := grpc.NewServer()
	metricsServer := NewMetricsServer(store, logger, trustedSubnet)
	proto.RegisterMetricsServer(server, metricsServer)

	logger.Infow("Starting gRPC server", "address", addr)

	go func() {
		if err := server.Serve(listener); err != nil {
			logger.Errorw("gRPC server error", "error", err)
		}
	}()

	return server, nil
}
