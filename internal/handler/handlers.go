package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/levinOo/go-metrics-project/internal/logger"
	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/levinOo/go-metrics-project/internal/repository"
	"go.uber.org/zap"
)

func NewRouter(storage repository.Storage, sugar *zap.SugaredLogger, cfgAddrDB string) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/", LoggerFuncServer(GetListHandler(storage), sugar))
	r.Get("/ping", LoggerFuncServer(PingHandler(storage), sugar))

	r.Route("/update", func(r chi.Router) {
		r.Post("/", LoggerFuncServer(DecompressMiddleware(UpdateJSONHandler(storage)), sugar))
		r.Post("/{typeMetric}/{metric}/{value}", LoggerFuncServer(UpdateValueHandler(storage, sugar), sugar))
	})

	r.Route("/value", func(r chi.Router) {
		r.Get("/{typeMetric}/{metric}", LoggerFuncServer(GetValueHandler(storage), sugar))
		r.Post("/", LoggerFuncServer(DecompressMiddleware(GetJSONHandler(storage)), sugar))
	})

	return r
}

func LoggerFuncServer(h http.Handler, sugar *zap.SugaredLogger) http.HandlerFunc {
	logFn := func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()

		responseData := &logger.ResponseData{
			Size:   0,
			Status: 0,
		}
		lw := logger.LoggingRW{
			ResponseWriter: rw,
			ResponseData:   responseData,
		}

		h.ServeHTTP(&lw, r)

		dur := time.Since(start)

		sugar.Infoln(
			"uri", r.RequestURI,
			"method", r.Method,
			"duration", dur,
			"status", responseData.Status,
			"size", responseData.Size,
		)
	}
	return http.HandlerFunc(logFn)
}

func DecompressMiddleware(h http.Handler) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(rw, "Failed to decompress gzip body", http.StatusBadRequest)
				return
			}
			defer gz.Close()

			body, err := io.ReadAll(gz)
			if err != nil {
				http.Error(rw, "Failed to read decompressed body", http.StatusInternalServerError)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
		h.ServeHTTP(rw, r)
	}
}

func PingHandler(dbConn repository.Storage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := dbConn.Ping(ctx)
		if err != nil {
			http.Error(rw, "No connection with Database", http.StatusInternalServerError)
			return
		}

		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("Database is reachable"))
	}
}

func UpdateValueHandler(storage repository.Storage, sugar *zap.SugaredLogger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		nameMetric := chi.URLParam(r, "metric")
		valueMetric := chi.URLParam(r, "value")
		typeMetric := chi.URLParam(r, "typeMetric")

		if nameMetric == "" {
			http.Error(rw, "Metric is empty", http.StatusNotFound)
			return
		}

		switch typeMetric {
		case "gauge":
			valueGauge, err := strconv.ParseFloat(valueMetric, 64)
			if err != nil {
				http.Error(rw, "Invalid type of value", http.StatusBadRequest)
				return
			}
			storage.SetGauge(nameMetric, repository.Gauge(valueGauge))
			sugar.Debugw("Set gauge metric", "name", nameMetric, "value", valueGauge)
		case "counter":
			valueCounter, err := strconv.ParseInt(valueMetric, 10, 64)
			if err != nil {
				http.Error(rw, "Invalid type of value", http.StatusBadRequest)
				return
			}
			storage.SetCounter(nameMetric, repository.Counter(valueCounter))
			sugar.Debugw("Set counter metric", "name", nameMetric, "value", valueCounter)
		default:
			http.Error(rw, "Unknown type of metric", http.StatusBadRequest)
			return
		}

		rw.WriteHeader(http.StatusOK)
		_, err := rw.Write([]byte("OK"))
		if err != nil {
			log.Printf("write status code error: %v", err)
		}
	}
}

func UpdateJSONHandler(storage repository.Storage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var metric models.Metrics

		err := json.NewDecoder(r.Body).Decode(&metric)
		if err != nil {
			http.Error(rw, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		switch metric.MType {
		case "gauge":
			storage.SetGauge(metric.ID, repository.Gauge(*metric.Value))
		case "counter":
			storage.SetCounter(metric.ID, repository.Counter(*metric.Delta))
		default:
			http.Error(rw, "Unknown type of metric", http.StatusBadRequest)
			return
		}

		accept := r.Header.Get("Accept")

		if strings.Contains(accept, "application/json") {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)

			response := map[string]string{"status": "ok"}
			if err := json.NewEncoder(rw).Encode(response); err != nil {
				log.Printf("json encode error: %v", err)
			}

		} else {
			rw.Header().Set("Content-Type", "text/html")
			rw.WriteHeader(http.StatusOK)

			_, err = rw.Write([]byte("<html><body><h1>OK</h1></body></html>"))
			if err != nil {
				log.Printf("write html error: %v", err)
			}
		}
	}
}

func GetJSONHandler(storage repository.Storage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var metric models.Metrics

		err := json.NewDecoder(r.Body).Decode(&metric)
		if err != nil {
			http.Error(rw, "", http.StatusBadRequest)
			return
		}

		switch metric.MType {
		case "gauge":
			val, err := storage.GetGauge(metric.ID)
			if err != nil {
				log.Printf("write error: %v", err)
				rw.WriteHeader(http.StatusNotFound)
				return
			}
			metric.Value = new(float64)
			*metric.Value = float64(val)

		case "counter":
			val, err := storage.GetCounter(metric.ID)
			if err != nil {
				log.Printf("write error: %v", err)
				rw.WriteHeader(http.StatusNotFound)
				return
			}
			metric.Delta = new(int64)
			*metric.Delta = int64(val)

		default:
			http.Error(rw, "Unknown type of metric", http.StatusBadRequest)
			return
		}

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			rw.Header().Set("Content-Encoding", "gzip")
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)

			gz := gzip.NewWriter(rw)
			defer gz.Close()

			err := json.NewEncoder(gz).Encode(metric)
			if err != nil {
				log.Printf("response gzip encode error: %v", err)
			}
		} else {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)

			err = json.NewEncoder(rw).Encode(metric)
			if err != nil {
				log.Printf("response encode error: %v", err)
			}
		}
	}
}

func GetValueHandler(storage repository.Storage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		nameMetric := chi.URLParam(r, "metric")

		switch chi.URLParam(r, "typeMetric") {
		case "gauge":
			val, err := storage.GetGauge(nameMetric)
			if err != nil {
				log.Printf("write error: %v", err)
				rw.WriteHeader(http.StatusNotFound)
				return
			}
			rw.WriteHeader(http.StatusOK)
			_, err = rw.Write([]byte(fmt.Sprintf("%g", val)))
			if err != nil {
				log.Printf("write error: %v", err)
			}
		case "counter":
			val, err := storage.GetCounter(nameMetric)
			if err != nil {
				log.Printf("write error: %v", err)
				rw.WriteHeader(http.StatusNotFound)
				return
			}
			rw.WriteHeader(http.StatusOK)
			_, err = rw.Write([]byte(fmt.Sprintf("%d", val)))
			if err != nil {
				log.Printf("write error: %v", err)
			}
		default:
			http.Error(rw, "Unknown type of metric", http.StatusBadRequest)
			return
		}
	}
}

func GetListHandler(storage repository.Storage) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var sb strings.Builder

		accept := r.Header.Get("Accept")
		metrics, err := storage.GetAll()
		if err != nil {
			http.Error(rw, fmt.Sprintf("failed to get all metrics: %v", err), http.StatusInternalServerError)
		}

		if strings.Contains(accept, "text/html") {
			sb.WriteString("<html><body>")
			sb.WriteString("<h1>Metrics</h1>")

			gaugeCount := 0
			counterCount := 0

			for _, metric := range metrics {
				if metric.MType == "gauge" {
					gaugeCount++
				} else if metric.MType == "counter" {
					counterCount++
				}
			}

			if gaugeCount > 0 {
				sb.WriteString("<h2>Gauges</h2><ul>")
				for _, metric := range metrics {
					if metric.MType == "gauge" && metric.Value != nil {
						sb.WriteString(fmt.Sprintf("<li>%s: %f</li>", metric.ID, *metric.Value))
					}
				}
				sb.WriteString("</ul>")
			}

			if counterCount > 0 {
				sb.WriteString("<h2>Counters</h2><ul>")
				for _, metric := range metrics {
					if metric.MType == "counter" && metric.Delta != nil {
						sb.WriteString(fmt.Sprintf("<li>%s: %d</li>", metric.ID, *metric.Delta))
					}
				}
				sb.WriteString("</ul>")
			}

			sb.WriteString("</body></html>")
		} else {
			for _, metric := range metrics {
				if metric.MType == "gauge" && metric.Value != nil {
					sb.WriteString(fmt.Sprintf("%s: %f\n", metric.ID, *metric.Value))
				} else if metric.MType == "counter" && metric.Delta != nil {
					sb.WriteString(fmt.Sprintf("%s: %d\n", metric.ID, *metric.Delta))
				}
			}
		}

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			rw.Header().Set("Content-Encoding", "gzip")
			if strings.Contains(accept, "text/html") {
				rw.Header().Set("Content-Type", "text/html")
			} else {
				rw.Header().Set("Content-Type", "text/plain")
			}
			rw.WriteHeader(http.StatusOK)

			gz := gzip.NewWriter(rw)
			defer gz.Close()

			_, err := gz.Write([]byte(sb.String()))
			if err != nil {
				log.Printf("gzip write error: %v", err)
			}
		} else {
			if strings.Contains(accept, "text/html") {
				rw.Header().Set("Content-Type", "text/html")
			} else {
				rw.Header().Set("Content-Type", "text/plain")
			}

			_, err := rw.Write([]byte(sb.String()))
			if err != nil {
				log.Printf("write error: %v", err)
			}
		}
	}
}
