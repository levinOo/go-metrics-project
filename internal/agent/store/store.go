package store

import (
	"math/rand"
	"runtime"
)

type (
	Gauge   float64
	Counter int64
)

type Metrics struct {
	Alloc         Gauge
	BuckHashSys   Gauge
	Frees         Gauge
	GCCPUFraction Gauge
	GCSys         Gauge
	HeapAlloc     Gauge
	HeapIdle      Gauge
	HeapInuse     Gauge
	HeapObjects   Gauge
	HeapReleased  Gauge
	HeapSys       Gauge
	LastGC        Gauge
	Lookups       Gauge
	MCacheInuse   Gauge
	MCacheSys     Gauge
	MSpanInuse    Gauge
	MSpanSys      Gauge
	Mallocs       Gauge
	NextGC        Gauge
	NumForcedGC   Gauge
	NumGC         Gauge
	OtherSys      Gauge
	PauseTotalNs  Gauge
	StackInuse    Gauge
	StackSys      Gauge
	Sys           Gauge
	TotalAlloc    Gauge
	RandomValue   Gauge

	PollCount Counter
}

func NewMetricsStorage() *Metrics {
	return &Metrics{}
}

func (m *Metrics) ValuesGauge() []Gauge {
	return []Gauge{
		m.Alloc,
		m.BuckHashSys,
		m.Frees,
		m.GCCPUFraction,
		m.GCSys,
		m.HeapAlloc,
		m.HeapIdle,
		m.HeapInuse,
		m.HeapObjects,
		m.HeapReleased,
		m.HeapSys,
		m.LastGC,
		m.Lookups,
		m.MCacheInuse,
		m.MCacheSys,
		m.MSpanInuse,
		m.MSpanSys,
		m.Mallocs,
		m.NextGC,
		m.NumForcedGC,
		m.NumGC,
		m.OtherSys,
		m.PauseTotalNs,
		m.StackInuse,
		m.StackSys,
		m.Sys,
		m.TotalAlloc,
		m.RandomValue,
	}
}

func (m *Metrics) ValuesCounter() []Counter {
	return []Counter{m.PollCount}
}

func (m *Metrics) GaugeNames() []string {
	return []string{
		"Alloc",
		"BuckHashSys",
		"Frees",
		"GCCPUFraction",
		"GCSys",
		"HeapAlloc",
		"HeapIdle",
		"HeapInuse",
		"HeapObjects",
		"HeapReleased",
		"HeapSys",
		"LastGC",
		"Lookups",
		"MCacheInuse",
		"MCacheSys",
		"MSpanInuse",
		"MSpanSys",
		"Mallocs",
		"NextGC",
		"NumForcedGC",
		"NumGC",
		"OtherSys",
		"PauseTotalNs",
		"StackInuse",
		"StackSys",
		"Sys",
		"TotalAlloc",
		"RandomValue",
	}
}

func (m *Metrics) CounterNames() []string {
	return []string{
		"PollCount",
	}
}

func (m *Metrics) CollectMetrics() {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	m.Alloc = Gauge(stats.Alloc)
	m.BuckHashSys = Gauge(stats.BuckHashSys)
	m.Frees = Gauge(stats.Frees)
	m.GCCPUFraction = Gauge(stats.GCCPUFraction)
	m.GCSys = Gauge(stats.GCSys)
	m.HeapAlloc = Gauge(stats.HeapAlloc)
	m.HeapIdle = Gauge(stats.HeapIdle)
	m.HeapInuse = Gauge(stats.HeapInuse)
	m.HeapObjects = Gauge(stats.HeapObjects)
	m.HeapReleased = Gauge(stats.HeapReleased)
	m.HeapSys = Gauge(stats.HeapSys)
	m.LastGC = Gauge(stats.LastGC)
	m.Lookups = Gauge(stats.Lookups)
	m.MCacheInuse = Gauge(stats.MCacheInuse)
	m.MCacheSys = Gauge(stats.MCacheSys)
	m.MSpanInuse = Gauge(stats.MSpanInuse)
	m.MSpanSys = Gauge(stats.MSpanSys)
	m.Mallocs = Gauge(stats.Mallocs)
	m.NextGC = Gauge(stats.NextGC)
	m.NumForcedGC = Gauge(stats.NumForcedGC)
	m.NumGC = Gauge(stats.NumGC)
	m.OtherSys = Gauge(stats.OtherSys)
	m.PauseTotalNs = Gauge(stats.PauseTotalNs)
	m.StackInuse = Gauge(stats.StackInuse)
	m.StackSys = Gauge(stats.StackSys)
	m.Sys = Gauge(stats.Sys)
	m.TotalAlloc = Gauge(stats.TotalAlloc)
	m.PollCount++
	m.RandomValue = Gauge(rand.Float64())
}
