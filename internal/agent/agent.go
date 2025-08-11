package agent

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/go-resty/resty/v2"
	"github.com/levinOo/go-metrics-project/internal/agent/store"
	"github.com/levinOo/go-metrics-project/internal/models"
)

type Config struct {
	Addr         string `env:"ADDRESS"`
	PollInterval int    `env:"POLL_INTERVAL"`
	ReqInterval  int    `env:"REPORT_INTERVAL"`
}

func SendAllMetricsBatch(client *http.Client, endpoint string, m store.Metrics) error {
	metrics := m.ValuesAllTyped()
	var metricsList []models.Metrics

	for name, metric := range metrics {
		metricModel := models.Metrics{
			ID:    name,
			MType: metric.Type(),
		}

		var err error
		switch metric.Type() {
		case "gauge":
			metricModel.Value = new(float64)
			*metricModel.Value, err = strconv.ParseFloat(metric.String(), 64)
			if err != nil {
				return fmt.Errorf("failed to parse gauge value for %s: %w", name, err)
			}
		case "counter":
			metricModel.Delta = new(int64)
			*metricModel.Delta, err = strconv.ParseInt(metric.String(), 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse counter value for %s: %w", name, err)
			}
		default:
			return fmt.Errorf("unknown metric type: %s for metric %s", metric.Type(), name)
		}

		metricsList = append(metricsList, metricModel)
	}

	if len(metricsList) == 0 {
		log.Println("No metrics to send, skipping batch")
		return nil
	}

	return sendMetricsBatch(metricsList, endpoint)
}

func sendMetricsBatch(metrics []models.Metrics, endpoint string) error {
	if len(metrics) == 0 {
		return nil
	}

	url, err := url.JoinPath(endpoint, "updates")
	if err != nil {
		return fmt.Errorf("failed to join URL path: %w", err)
	}

	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	buffer, err := CompressData(data)
	if err != nil {
		return fmt.Errorf("failed to compress data: %w", err)
	}

	client := resty.New()
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetHeader("Content-Encoding", "gzip").
		SetHeader("Accept-Encoding", "gzip").
		SetBody(buffer).
		Post(url)

	if err != nil {
		return fmt.Errorf("failed to send batch request: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode(), resp.String())
	}

	return nil
}

func CompressData(data []byte) ([]byte, error) {
	var buffer bytes.Buffer

	w := gzip.NewWriter(&buffer)

	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}

	err = w.Close()
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func StartAgent() <-chan error {
	cfg := Config{}
	errCh := make(chan error)

	flag.StringVar(&cfg.Addr, "a", "localhost:8080", "Адрес сервера")
	flag.IntVar(&cfg.PollInterval, "p", 2, "Значение интервала обновления метрик в секундах")
	flag.IntVar(&cfg.ReqInterval, "r", 10, "Значение интервала отпрвки в секундах")
	flag.Parse()

	err := env.Parse(&cfg)
	if err != nil {
		errCh <- fmt.Errorf("ошибка парсинга ENV: %w", err)
		return errCh
	}

	m := store.NewMetricsStorage()
	endpoint := "http://" + cfg.Addr

	go func() {
		pollTicker := time.NewTicker(time.Second * time.Duration((cfg.PollInterval)))
		reqTicker := time.NewTicker(time.Second * time.Duration((cfg.ReqInterval)))

		for {
			select {
			case <-pollTicker.C:
				m.CollectMetrics()

			case <-reqTicker.C:
				var connRefusedErr = syscall.ECONNREFUSED
				err := SendAllMetricsBatch(&http.Client{}, endpoint, *m)

				if errors.Is(err, connRefusedErr) {
					intervals := []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}

					for i := 0; i < 3; i++ {
						log.Printf("Retry attempt %d after error: %v", i+1, err)
						time.Sleep(intervals[i])

						err = SendAllMetricsBatch(&http.Client{}, endpoint, *m)
						if err == nil {
							log.Printf("Success after %d retries", i+1)
							break
						}

						if !errors.Is(err, connRefusedErr) {
							break
						}
					}
				}

				if err != nil {
					log.Printf("Final sending metrics error: %v", err)
				}
			}
		}
	}()

	return errCh
}
