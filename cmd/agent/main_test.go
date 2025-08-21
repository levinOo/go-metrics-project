package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/levinOo/go-metrics-project/internal/agent"
	"github.com/levinOo/go-metrics-project/internal/agent/store"
	"github.com/levinOo/go-metrics-project/internal/models"
)

func TestCompressData(t *testing.T) {
	original := []byte(`{"test":"value"}`)
	compressed, err := agent.CompressData(original)
	if err != nil {
		t.Fatalf("CompressData error: %v", err)
	}

	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader error: %v", err)
	}
	defer r.Close()

	decoded, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("Error decompressing data: %v", err)
	}

	if !bytes.Equal(decoded, original) {
		t.Errorf("Decompressed data doesn't match original.\nGot: %s\nWant: %s", decoded, original)
	}
}

func TestSendAllMetricsBatch(t *testing.T) {
	expectedMetrics := map[string]bool{"Alloc": false, "PollCount": false}
	requestCount := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if !strings.HasSuffix(r.URL.Path, "/updates") {
			t.Errorf("expected path to end with /updates, got %s", r.URL.Path)
		}

		if r.Header.Get("Content-Encoding") != "gzip" {
			t.Errorf("expected Content-Encoding: gzip, got %s", r.Header.Get("Content-Encoding"))
		}

		rdr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Errorf("gzip.NewReader error: %v", err)
			return
		}
		defer rdr.Close()

		body, err := io.ReadAll(rdr)
		if err != nil {
			t.Errorf("read error: %v", err)
		}

		var metrics []models.Metrics
		if err := json.Unmarshal(body, &metrics); err != nil {
			t.Errorf("unmarshal error: %v", err)
		}

		for _, metric := range metrics {
			if _, exists := expectedMetrics[metric.ID]; exists {
				expectedMetrics[metric.ID] = true
			}
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	metrics := store.Metrics{
		Alloc:     store.Gauge(42.42),
		PollCount: store.Counter(7),
	}

	client := &http.Client{}
	err := agent.SendAllMetricsBatch(client, ts.URL, metrics, "")
	if err != nil {
		t.Errorf("SendAllMetricsBatch failed: %v", err)
	}

	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}

	for name, received := range expectedMetrics {
		if !received {
			t.Errorf("metric %s was not sent", name)
		}
	}
}
