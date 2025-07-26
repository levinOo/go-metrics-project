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

func TestSendMetric(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/update") {
			t.Errorf("expected path to end with /update, got %s", r.URL.Path)
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
			t.Errorf("error reading gzip body: %v", err)
		}

		var m models.Metrics
		if err := json.Unmarshal(body, &m); err != nil {
			t.Errorf("error unmarshalling metric: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	err := agent.SendMetric("gauge", "Alloc", "123.45", ts.URL)
	if err != nil {
		t.Errorf("SendMetric failed: %v", err)
	}

	err = agent.SendMetric("counter", "PollCount", "99", ts.URL)
	if err != nil {
		t.Errorf("SendMetric failed: %v", err)
	}
}

func TestSendAllMetrics(t *testing.T) {
	expected := map[string]bool{"Alloc": false, "PollCount": false}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		var m models.Metrics
		if err := json.Unmarshal(body, &m); err != nil {
			t.Errorf("unmarshal error: %v", err)
		}

		expected[m.ID] = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	metrics := store.Metrics{
		Alloc:     store.Gauge(42.42),
		PollCount: store.Counter(7),
	}

	client := &http.Client{}
	err := agent.SendAllMetrics(client, ts.URL, metrics)
	if err != nil {
		t.Errorf("SendAllMetrics failed: %v", err)
	}

	for name, received := range expected {
		if !received {
			t.Errorf("metric %s was not sent", name)
		}
	}
}
