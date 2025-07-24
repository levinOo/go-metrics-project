package agent

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
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

func SendMetric(metricType, metricName, metricValue, endpoint string) error {
	url, err := url.JoinPath(endpoint, "update")
	if err != nil {
		return err
	}

	stor := models.Metrics{
		ID:    metricName,
		MType: metricType,
	}

	switch stor.MType {
	case "gauge":
		stor.Value = new(float64)
		*stor.Value, err = strconv.ParseFloat(metricValue, 64)
		if err != nil {
			return err
		}
	case "counter":
		stor.Delta = new(int64)
		*stor.Delta, err = strconv.ParseInt(metricValue, 10, 64)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown metric type: %s", metricType)
	}

	data, err := json.Marshal(stor)
	if err != nil {
		return err
	}

	buffer, err := CompressData(data)
	if err != nil {
		return err
	}

	client := resty.New()
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetHeader("Content-Encoding", "deflate").
		SetHeader("Accept-Encoding", "gzip").
		SetBody(buffer).
		Post(url)

	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode(), resp.String())
	}

	contentType := resp.Header().Get("Content-Type")
	if contentType != "" && contentType != "application/json" {
		log.Printf("Warning: expected Content-Type application/json, got %s", contentType)
	}

	return nil

}

func SendAllMetrics(client *http.Client, endpoint string, m store.Metrics) error {

	v := reflect.ValueOf(m)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		var metricType string
		var metricValue string

		switch value.Kind() {
		case reflect.Float64:
			metricType = "gauge"
			metricValue = strconv.FormatFloat(value.Float(), 'f', -1, 64)
		case reflect.Int64:
			metricType = "counter"
			metricValue = strconv.FormatInt(value.Int(), 10)
		default:
			continue
		}

		metricName := field.Name
		err := SendMetric(metricType, metricName, metricValue, endpoint)
		if err != nil {
			return err
		}
	}
	return nil
}

func CompressData(data []byte) ([]byte, error) {
	var buffer bytes.Buffer

	w, err := flate.NewWriter(&buffer, flate.BestCompression)
	if err != nil {
		return nil, err
	}

	_, err = w.Write(data)
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
				if err := SendAllMetrics(&http.Client{}, endpoint, *m); err != nil {
					log.Printf("Sending metrics error: %v", err)
					continue
				}

			}
		}
	}()

	return errCh
}
