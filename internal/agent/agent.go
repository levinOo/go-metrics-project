package agent

import (
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
	PollInterval int    `env:"REPORT_INTERVAL"`
	ReqInterval  int    `env:"POLL_INTERVAL"`
}

func SendMetric(metricType, metricName, metricValue, endpoint string) error {
	url, err := url.JoinPath(endpoint, "update")
	if err != nil {
		return err
	}

	var metric models.Metrics
	metric.ID = metricName
	metric.MType = metricType

	switch metric.MType {
	case "gauge":
		val, err := strconv.ParseFloat(metricValue, 64)
		if err != nil {
			return err
		}
		metric.Value = &val
	case "counter":
		val, err := strconv.ParseInt(metricValue, 10, 64)
		if err != nil {
			return err
		}
		metric.Delta = &val
	default:
		return fmt.Errorf("unsupported metric type: %s", metricType)
	}

	client := resty.New()
	client.SetHeader("Content-Type", "application/json")

	_, err = client.R().
		SetBody(metric).
		Post(url)
	return err
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
		}

		metricName := field.Name
		err := SendMetric(metricType, metricName, metricValue, endpoint)
		if err != nil {
			return err
		}
	}
	return nil
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
