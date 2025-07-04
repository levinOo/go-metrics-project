package main

import (
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

type (
	gauge   float64
	counter int64
)

type Metrics struct {
	Alloc         gauge
	BuckHashSys   gauge
	Frees         gauge
	GCCPUFraction gauge
	GCSys         gauge
	HeapAlloc     gauge
	HeapIdle      gauge
	HeapInuse     gauge
	HeapObjects   gauge
	HeapReleased  gauge
	HeapSys       gauge
	LastGC        gauge
	Lookups       gauge
	MCacheInuse   gauge
	MCacheSys     gauge
	MSpanInuse    gauge
	MSpanSys      gauge
	Mallocs       gauge
	NextGC        gauge
	NumForcedGC   gauge
	NumGC         gauge
	OtherSys      gauge
	PauseTotalNs  gauge
	StackInuse    gauge
	StackSys      gauge
	Sys           gauge
	TotalAlloc    gauge
	PollCount     counter
	RandomValue   counter
}

func (m *Metrics) CollectMetrics() {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	m.Alloc = gauge(stats.Alloc)
	m.BuckHashSys = gauge(stats.BuckHashSys)
	m.Frees = gauge(stats.Frees)
	m.GCCPUFraction = gauge(stats.GCCPUFraction)
	m.GCSys = gauge(stats.GCSys)
	m.HeapAlloc = gauge(stats.HeapAlloc)
	m.HeapIdle = gauge(stats.HeapIdle)
	m.HeapInuse = gauge(stats.HeapInuse)
	m.HeapObjects = gauge(stats.HeapObjects)
	m.HeapReleased = gauge(stats.HeapReleased)
	m.HeapSys = gauge(stats.HeapSys)
	m.LastGC = gauge(stats.LastGC)
	m.Lookups = gauge(stats.Lookups)
	m.MCacheInuse = gauge(stats.MCacheInuse)
	m.MCacheSys = gauge(stats.MCacheSys)
	m.MSpanInuse = gauge(stats.MSpanInuse)
	m.MSpanSys = gauge(stats.MSpanSys)
	m.Mallocs = gauge(stats.Mallocs)
	m.NextGC = gauge(stats.NextGC)
	m.NumForcedGC = gauge(stats.NumForcedGC)
	m.NumGC = gauge(stats.NumGC)
	m.OtherSys = gauge(stats.OtherSys)
	m.PauseTotalNs = gauge(stats.PauseTotalNs)
	m.StackInuse = gauge(stats.StackInuse)
	m.StackSys = gauge(stats.StackSys)
	m.Sys = gauge(stats.Sys)
	m.TotalAlloc = gauge(stats.TotalAlloc)
	m.PollCount++
	m.RandomValue = counter(rand.Intn(1000))
}

func SendMetrics(client *http.Client, endpoint string, m *Metrics) {
	sendMetric := func(metricType, metricName, metricValue string) {
		url := endpoint + "/update/" + metricType + "/" + metricName + "/" + metricValue

		client := resty.New()

		client.SetHeader("Content-Type", "text/plain")

		_, err := client.R().
			Post(url)
		if err != nil {
			log.Printf("Ошибка отправки метрики %s: %v", metricName, err)
		}
	}

	sendMetric("gauge", "Alloc", strconv.FormatFloat(float64(m.Alloc), 'f', -1, 64))
	sendMetric("gauge", "BuckHashSys", strconv.FormatFloat(float64(m.BuckHashSys), 'f', -1, 64))
	sendMetric("gauge", "Frees", strconv.FormatFloat(float64(m.Frees), 'f', -1, 64))
	sendMetric("gauge", "GCCPUFraction", strconv.FormatFloat(float64(m.GCCPUFraction), 'f', -1, 64))
	sendMetric("gauge", "GCSys", strconv.FormatFloat(float64(m.GCSys), 'f', -1, 64))
	sendMetric("gauge", "HeapAlloc", strconv.FormatFloat(float64(m.HeapAlloc), 'f', -1, 64))
	sendMetric("gauge", "HeapIdle", strconv.FormatFloat(float64(m.HeapIdle), 'f', -1, 64))
	sendMetric("gauge", "HeapInuse", strconv.FormatFloat(float64(m.HeapInuse), 'f', -1, 64))
	sendMetric("gauge", "HeapObjects", strconv.FormatFloat(float64(m.HeapObjects), 'f', -1, 64))
	sendMetric("gauge", "HeapReleased", strconv.FormatFloat(float64(m.HeapReleased), 'f', -1, 64))
	sendMetric("gauge", "HeapSys", strconv.FormatFloat(float64(m.HeapSys), 'f', -1, 64))
	sendMetric("gauge", "LastGC", strconv.FormatFloat(float64(m.LastGC), 'f', -1, 64))
	sendMetric("gauge", "Lookups", strconv.FormatFloat(float64(m.Lookups), 'f', -1, 64))
	sendMetric("gauge", "MCacheInuse", strconv.FormatFloat(float64(m.MCacheInuse), 'f', -1, 64))
	sendMetric("gauge", "MCacheSys", strconv.FormatFloat(float64(m.MCacheSys), 'f', -1, 64))
	sendMetric("gauge", "MSpanInuse", strconv.FormatFloat(float64(m.MSpanInuse), 'f', -1, 64))
	sendMetric("gauge", "MSpanSys", strconv.FormatFloat(float64(m.MSpanSys), 'f', -1, 64))
	sendMetric("gauge", "Mallocs", strconv.FormatFloat(float64(m.Mallocs), 'f', -1, 64))
	sendMetric("gauge", "NextGC", strconv.FormatFloat(float64(m.NextGC), 'f', -1, 64))
	sendMetric("gauge", "NumForcedGC", strconv.FormatFloat(float64(m.NumForcedGC), 'f', -1, 64))
	sendMetric("gauge", "NumGC", strconv.FormatFloat(float64(m.NumGC), 'f', -1, 64))
	sendMetric("gauge", "OtherSys", strconv.FormatFloat(float64(m.OtherSys), 'f', -1, 64))
	sendMetric("gauge", "PauseTotalNs", strconv.FormatFloat(float64(m.PauseTotalNs), 'f', -1, 64))
	sendMetric("gauge", "StackInuse", strconv.FormatFloat(float64(m.StackInuse), 'f', -1, 64))
	sendMetric("gauge", "StackSys", strconv.FormatFloat(float64(m.StackSys), 'f', -1, 64))
	sendMetric("gauge", "Sys", strconv.FormatFloat(float64(m.Sys), 'f', -1, 64))
	sendMetric("gauge", "TotalAlloc", strconv.FormatFloat(float64(m.TotalAlloc), 'f', -1, 64))
	sendMetric("counter", "PollCount", strconv.FormatInt(int64(m.PollCount), 10))
	sendMetric("counter", "RandomValue", strconv.FormatInt(int64(m.RandomValue), 10))
}

func NewMetricsStorage() *Metrics {
	return &Metrics{}
}

func main() {

	m := NewMetricsStorage()
	endpoint := "http://localhost:8080"

	pollTicker := time.Second * 2
	reqTicker := time.Second * 10

	pollTime := time.Now()
	reqTime := time.Now()

	for {
		now := time.Now()

		if now.Sub(pollTime) >= pollTicker {
			m.CollectMetrics()
			pollTime = now
		}

		if now.Sub(reqTime) >= reqTicker {
			SendMetrics(&http.Client{}, endpoint, m)
			reqTime = now
		}
		time.Sleep(500 * time.Millisecond)
	}
}
