// Package grpc предоставляет gRPC-клиент для отправки метрик на сервер.
package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/levinOo/go-metrics-project/internal/models"
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

	// Создаем запрос - используем Open Struct API (прямой доступ к полям)
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
	var protoMetrics []*proto.Metric

	// Обрабатываем различные типы входных данных
	switch v := metricsInterface.(type) {
	case []interface{}:
		// Если передан срез интерфейсов
		for _, item := range v {
			if metric, ok := item.(models.Metrics); ok {
				pm, err := convertSingleMetric(metric)
				if err != nil {
					return nil, err
				}
				protoMetrics = append(protoMetrics, pm)
			}
		}
	case []models.Metrics:
		// Если передан срез models.Metrics напрямую
		for _, metric := range v {
			pm, err := convertSingleMetric(metric)
			if err != nil {
				return nil, err
			}
			protoMetrics = append(protoMetrics, pm)
		}
	default:
		return nil, fmt.Errorf("unsupported metrics type: %T", metricsInterface)
	}

	return protoMetrics, nil
}

// convertSingleMetric преобразует одну метрику из models.Metrics в proto.Metric
// Используем Open Struct API (protogen:"open.v1") - прямой доступ к экспортированным полям
func convertSingleMetric(metric models.Metrics) (*proto.Metric, error) {
	pm := &proto.Metric{
		Id: metric.ID,
	}

	switch metric.MType {
	case "gauge":
		if metric.Value == nil {
			return nil, fmt.Errorf("gauge metric %s has nil value", metric.ID)
		}
		pm.Type = proto.Metric_GAUGE
		pm.Value = *metric.Value

	case "counter":
		if metric.Delta == nil {
			return nil, fmt.Errorf("counter metric %s has nil delta", metric.ID)
		}
		pm.Type = proto.Metric_COUNTER
		pm.Delta = *metric.Delta

	default:
		return nil, fmt.Errorf("unknown metric type: %s for metric %s", metric.MType, metric.ID)
	}

	return pm, nil
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

	// Возвращаем default IP для локальной разработки
	return defaultTrustedIP, nil
}
