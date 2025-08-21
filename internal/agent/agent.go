package agent

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/levinOo/go-metrics-project/internal/agent/store"
	"github.com/levinOo/go-metrics-project/internal/models"
)

type Config struct {
	Addr         string `env:"ADDRESS"`
	Key          string `env:"KEY"`
	PollInterval int    `env:"POLL_INTERVAL"`
	ReqInterval  int    `env:"REPORT_INTERVAL"`
}

func SendAllMetricsBatch(client *http.Client, endpoint string, m store.Metrics, key string) error {
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

	return sendMetricsBatch(metricsList, endpoint, key)
}

func sendMetricsBatch(metrics []models.Metrics, endpoint string, key string) error {
	url, err := url.JoinPath(endpoint, "updates")
	if err != nil {
		return fmt.Errorf("failed to join URL path: %w", err)
	}

	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	var hashString string
	if key != "" {
		hashString = calculateSHA256Hash(data)
	}

	buffer, err := CompressData(data)
	if err != nil {
		return fmt.Errorf("failed to compress data: %w", err)
	}

	client := retryablehttp.NewClient()
	client.RetryMax = 3
	client.RetryWaitMax = 3 * time.Second
	client.RetryWaitMin = 1 * time.Second
	client.Backoff = customBackoff

	client.Backoff = customBackoff

	req, err := retryablehttp.NewRequest("POST", url, buffer)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	if hashString != "" {
		req.Header.Set("HashSHA256", hashString)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send batch request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

func customBackoff(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
	delays := []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}

	indx := attemptNum
	if indx >= len(delays) {
		indx = len(delays) - 1
	}

	return delays[indx]
}

func calculateSHA256Hash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
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
	flag.StringVar(&cfg.Key, "k", "", "Ключ шифрования")
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
				err := SendAllMetricsBatch(&http.Client{}, endpoint, *m, cfg.Key)

				if err != nil {
					log.Printf("Final sending metrics error: %v", err)
				}
			}
		}
	}()

	return errCh
}
