package agent

import (
	"flag"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/levinOo/go-metrics-project/internal/agent/store"
)

func SendMetric(metricType, metricName, metricValue, endpoint string) error {
	url, err := url.JoinPath(endpoint, "update", metricType, metricName, metricValue)
	if err != nil {
		return err
	}

	client := resty.New()
	client.SetHeader("Content-Type", "text/plain")
	_, err = client.R().
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

	addr := flag.String("a", "localhost:8080", "Адрес сервера")
	pollInterval := flag.Int("p", 2, "Значение интервала обновления метрик в секундах")
	reqInterval := flag.Int("r", 10, "Значение интервала отпрвки в секундах")
	flag.Parse()

	m := store.NewMetricsStorage()
	endpoint := "http://" + *addr

	errCh := make(chan error)

	go func() {
		pollTicker := time.NewTicker(time.Second * time.Duration(*pollInterval))
		reqTicker := time.NewTicker(time.Second * time.Duration(*reqInterval))

		for {
			select {
			case <-pollTicker.C:
				m.CollectMetrics()
			case <-reqTicker.C:
				if err := SendAllMetrics(&http.Client{}, endpoint, *m); err != nil {
					errCh <- err
					return
				}

			}
		}
	}()

	return errCh
}
