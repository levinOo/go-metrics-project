package store

import (
	"log"
	"math/rand"
	"runtime"
	"strconv"

	"github.com/shirou/gopsutil/mem"
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

	TotalMemory     Gauge
	FreeMemory      Gauge
	CPUutilization1 Gauge

	PollCount Counter
}

func NewMetricsStorage() *Metrics {
	return &Metrics{}
}

type Metric interface {
	String() string
	Type() string
}

func (g Gauge) String() string {
	return strconv.FormatFloat(float64(g), 'f', -1, 64)
}

func (g Gauge) Type() string {
	return "gauge"
}

func (c Counter) String() string {
	return strconv.FormatInt(int64(c), 10)
}

func (c Counter) Type() string {
	return "counter"
}

func (m *Metrics) ValuesAllTyped() map[string]Metric {
	result := make(map[string]Metric)
	for name, val := range m.ValuesGauge() {
		result[name] = val
	}
	for name, val := range m.ValuesCounter() {
		result[name] = val
	}
	return result
}

func (m *Metrics) ValuesGauge() map[string]Metric {
	return map[string]Metric{
		"Alloc":         m.Alloc,
		"BuckHashSys":   m.BuckHashSys,
		"Frees":         m.Frees,
		"GCCPUFraction": m.GCCPUFraction,
		"GCSys":         m.GCSys,
		"HeapAlloc":     m.HeapAlloc,
		"HeapIdle":      m.HeapIdle,
		"HeapInuse":     m.HeapInuse,
		"HeapObjects":   m.HeapObjects,
		"HeapReleased":  m.HeapReleased,
		"HeapSys":       m.HeapSys,
		"LastGC":        m.LastGC,
		"Lookups":       m.Lookups,
		"MCacheInuse":   m.MCacheInuse,
		"MCacheSys":     m.MCacheSys,
		"MSpanInuse":    m.MSpanInuse,
		"MSpanSys":      m.MSpanSys,
		"Mallocs":       m.Mallocs,
		"NextGC":        m.NextGC,
		"NumForcedGC":   m.NumForcedGC,
		"NumGC":         m.NumGC,
		"OtherSys":      m.OtherSys,
		"PauseTotalNs":  m.PauseTotalNs,
		"StackInuse":    m.StackInuse,
		"StackSys":      m.StackSys,
		"Sys":           m.Sys,
		"TotalAlloc":    m.TotalAlloc,
		"RandomValue":   m.RandomValue,
	}
}

func (m *Metrics) ValuesCounter() map[string]Metric {
	return map[string]Metric{
		"PollCount": m.PollCount,
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

func (m *Metrics) CollectAdditionalMetrics() {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	memStat, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("Error collecting memory metrics: %v", err)
	}

	m.TotalMemory = Gauge(memStat.Total)
	m.FreeMemory = Gauge(memStat.Available)
	m.CPUutilization1 = Gauge(runtime.NumCPU())
}
