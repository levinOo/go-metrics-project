// Package client предоставляет gRPC-клиент для отправки метрик на сервер.
package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/levinOo/go-metrics-project/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	// defaultTrustedIP используется как fallback IP-адрес агента,
	// если не удалось определить реальный не-loopback IPv4 адрес.
	// Используется для локальной разработки и тестирования.
	defaultTrustedIP = "127.0.0.1"
)

// GRPCClient предоставляет интерфейс для отправки метрик через gRPC.
type GRPCClient struct {
	conn   *grpc.ClientConn
	client proto.MetricsClient
	addr   string
}

// NewGRPCClient создает новый gRPC-клиент и подключается к серверу.
func NewGRPCClient(addr string) (*GRPCClient, error) {
	// Используем новый API grpc.NewClient вместо устаревшего grpc.Dial
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for %s: %w", addr, err)
	}

	return &GRPCClient{
		conn:   conn,
		client: proto.NewMetricsClient(conn),
		addr:   addr,
	}, nil
}

// SendMetrics отправляет батч метрик на gRPC-сервер.
// metrics параметр - это срез моделей Metrics из internal/models.
// agentIP - IP-адрес агента для передачи в метаданных.
func (c *GRPCClient) SendMetrics(ctx context.Context, metrics interface{}, agentIP string) error {
	// Преобразование внутренних моделей в proto модели
	protoMetrics, err := convertMetricsToProto(metrics)
	if err != nil {
		return fmt.Errorf("failed to convert metrics: %w", err)
	}

	// Создание контекста с метаданными (x-real-ip)
	md := metadata.New(map[string]string{
		"x-real-ip": agentIP,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	req := &proto.UpdateMetricsRequest{
		Metrics: protoMetrics,
	}

	_, err = c.client.UpdateMetrics(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send metrics: %w", err)
	}

	return nil
}

// Close закрывает соединение с gRPC-сервером.
func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

// convertMetricsToProto преобразует внутренние модели метрик в proto модели.
func convertMetricsToProto(metricsInterface interface{}) ([]*proto.Metric, error) {
	// Пытаемся получить срез метрик из интерфейса
	// Эта функция работает с внутренними моделями models.Metrics
	// которые передаются из агента

	type MetricModel struct {
		ID    string
		MType string
		Delta *int64
		Value *float64
	}

	var protoMetrics []*proto.Metric

	// Используем type assertion для обработки различных типов
	switch v := metricsInterface.(type) {
	case []interface{}:
		for _, item := range v {
			if metric, ok := item.(MetricModel); ok {
				pm := &proto.Metric{
					Id: metric.ID,
				}

				switch metric.MType {
				case "gauge":
					pm.Type = proto.Metric_GAUGE
					if metric.Value != nil {
						pm.Value = *metric.Value
					}
				case "counter":
					pm.Type = proto.Metric_COUNTER
					if metric.Delta != nil {
						pm.Delta = *metric.Delta
					}
				default:
					return nil, fmt.Errorf("unknown metric type: %s", metric.MType)
				}

				protoMetrics = append(protoMetrics, pm)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported metrics type")
	}

	return protoMetrics, nil
}

// GetAgentIP получает IP-адрес агента для передачи в метаданных.
// Возвращает первый найденный не-loopback IPv4 адрес.
// Если такой адрес не найден, возвращает defaultTrustedIP для локального использования.
func GetAgentIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return defaultTrustedIP, nil
}
