package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/levinOo/go-metrics-project/internal/agent/config"
	"github.com/levinOo/go-metrics-project/internal/agent/store"
	"github.com/levinOo/go-metrics-project/internal/cryptoutil"
	"github.com/levinOo/go-metrics-project/internal/models"
)

func SendAllMetricsBatch(client *http.Client, endpoint string, m store.Metrics, key string, rateLimit int, publicKey *rsa.PublicKey) error {
	metrics := m.ValuesAllTyped()
	var metricsList []models.Metrics

	inputCh := make(chan models.Metrics)
	errCh := make(chan error)
	go func() {
		defer close(inputCh)
		for name, metric := range metrics {
			inputCh <- models.Metrics{
				ID:    name,
				MType: metric.Type(),
			}
		}
	}()

	var wg sync.WaitGroup
	var mu sync.Mutex

	for w := 1; w <= rateLimit; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for metric := range inputCh {
				var err error
				switch metric.MType {
				case "gauge":
					metric.Value = new(float64)
					*metric.Value, err = strconv.ParseFloat(metrics[metric.ID].String(), 64)
					if err != nil {
						errCh <- fmt.Errorf("failed to parse gauge value for %s: %w", metric.ID, err)
						return
					}
				case "counter":
					metric.Delta = new(int64)
					*metric.Delta, err = strconv.ParseInt(metrics[metric.ID].String(), 10, 64)
					if err != nil {
						errCh <- fmt.Errorf("failed to parse counter value for %s: %w", metric.ID, err)
						return
					}
				default:
					errCh <- fmt.Errorf("unknown metric type: %s for metric %s", metric.MType, metric.ID)
					return
				}

				mu.Lock()
				metricsList = append(metricsList, metric)
				mu.Unlock()
			}
		}()
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return sendMetricsBatch(metricsList, endpoint, key, publicKey)
}

func sendMetricsBatch(metrics []models.Metrics, endpoint string, key string, publicKey *rsa.PublicKey) error {
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
		return fmt.Errorf("failed to compress: %w", err)
	}

	var hashString string
	if key != "" {
		hashString = calculateSHA256Hash(buffer, key)
	}

	if publicKey != nil {
		buffer, err = cryptoutil.EncryptDataHybrid(publicKey, buffer)
		if err != nil {
			return fmt.Errorf("failed to encrypt: %w", err)
		}
	}

	client := retryablehttp.NewClient()
	client.RetryMax = 3
	client.RetryWaitMax = 3 * time.Second
	client.RetryWaitMin = 1 * time.Second
	client.Backoff = customBackoff

	req, err := retryablehttp.NewRequest("POST", url, bytes.NewReader(buffer))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

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

func calculateSHA256Hash(data []byte, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write(data)
	hash := h.Sum(nil)
	return hex.EncodeToString(hash)
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

type Config struct {
	Addr          string `env:"ADDRESS"`
	Key           string `env:"KEY"`
	PollInterval  int    `env:"POLL_INTERVAL"`
	ReqInterval   int    `env:"REPORT_INTERVAL"`
	RateLimit     int    `env:"RATE_LIMIT"`
	CryptoKeyPath string `env:"CRYPTO_KEY"`
}

func StartAgent() <-chan error {
	cfg := config.NewConfig()
	config.GetAgentConfig(cfg)

	errCh := make(chan error)

	publicKey, err := cryptoutil.LoadPublicKey(cfg.CryptoKeyPath)
	if err != nil {
		errCh <- fmt.Errorf("ошибка создвния Public key: %w", err)
		return errCh
	}

	m := store.NewMetricsStorage()
	endpoint := "http://" + cfg.Addr

	semaphore := make(chan struct{}, cfg.RateLimit)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	go func() {
		pollTicker := time.NewTicker(time.Second * time.Duration((cfg.PollInterval)))
		reqTicker := time.NewTicker(time.Second * time.Duration((cfg.ReqInterval)))

		defer pollTicker.Stop()
		defer reqTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-pollTicker.C:
				wg.Add(1)
				go func() {
					defer wg.Done()
					m.CollectMetrics()
				}()

			case <-reqTicker.C:
				wg.Add(1)
				go func() {
					defer wg.Done()
					semaphore <- struct{}{}
					defer func() { <-semaphore }()

					err := SendAllMetricsBatch(&http.Client{}, endpoint, *m, cfg.Key, cfg.RateLimit, publicKey)

					if err != nil {
						log.Printf("Final sending metrics error: %v", err)
					}
				}()
			}
		}
	}()

	for {
		<-quit
		log.Printf("Running graceful shutdown")
		cancel()
		break
	}

	go func() {
		wg.Wait()
		close(errCh)
		log.Printf("Graceful shutdown completed")
	}()

	return errCh
}
