package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/levinOo/go-metrics-project/internal/config"
	"github.com/levinOo/go-metrics-project/internal/logger"
	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/levinOo/go-metrics-project/internal/repository"
	"go.uber.org/zap"
)

func NewRouter(storage repository.Storage, sugar *zap.SugaredLogger, cfg config.Config) *chi.Mux {
	r := chi.NewRouter()

	r.Use(LoggerMiddleware(sugar))
	r.Use(DecompressMiddleware())
	r.Use(DecryptMiddleware(cfg.Key))

	r.Get("/", GetListHandler(storage))
	r.Get("/ping", PingHandler(storage))

	r.Post("/updates", UpdatesValuesHandler(storage, cfg.Key, cfg.AuditFile, cfg.AuditURL))
	r.Post("/updates/", UpdatesValuesHandler(storage, cfg.Key, cfg.AuditFile, cfg.AuditURL))

	r.Route("/update", func(r chi.Router) {
		r.Post("/", UpdateJSONHandler(storage, cfg.Key))
		r.Post("/{typeMetric}/{metric}/{value}", UpdateValueHandler(storage, sugar))
	})

	r.Post("/value/", GetJSONHandler(storage, cfg.Key))
	r.Route("/value", func(r chi.Router) {
		r.Get("/{typeMetric}/{metric}", GetValueHandler(storage))
		r.Post("/", GetJSONHandler(storage, cfg.Key))
	})

	return r
}

func LoggerMiddleware(sugar *zap.SugaredLogger) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
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
		})
	}
}

func DecompressMiddleware() func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
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
		})
	}
}

func DecryptMiddleware(key string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			receivedHash := r.Header.Get("Hash")
			if receivedHash == "" {
				receivedHash = r.Header.Get("HashSHA256")
			}

			if receivedHash == "" || receivedHash == "none" {
				h.ServeHTTP(rw, r)
				return
			}

			if key != "" {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					log.Println("error reading r.Body:", err)
					http.Error(rw, "read body error", http.StatusBadRequest)
					return
				}

				r.Body = io.NopCloser(bytes.NewBuffer(body))

				sig, err := hex.DecodeString(receivedHash)
				if err != nil {
					log.Println("bad hash format")
					http.Error(rw, "bad hash format", http.StatusBadRequest)
					return
				}

				hash := hmac.New(sha256.New, []byte(key))
				hash.Write(body)
				expectedSig := hash.Sum(nil)

				if !hmac.Equal(expectedSig, sig) {
					log.Println("Incorrect hash")
					log.Printf("Expected: %x, Received: %x\n", expectedSig, sig)
					http.Error(rw, "invalid hash", http.StatusBadRequest)
					return
				}
			}

			h.ServeHTTP(rw, r)
		})
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

func UpdatesValuesHandler(storage repository.Storage, key, path, url string) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var metrics models.ListMetrics

		err := json.NewDecoder(r.Body).Decode(&metrics)
		if err != nil {
			http.Error(rw, "invalid JSON format", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		err = storage.InsertMetricsBatch(metrics)
		if err != nil {
			http.Error(rw, "internal server error", http.StatusInternalServerError)
			return
		}

		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		metrics.NewAuditEvent(path, url, ip)

		response := map[string]string{"status": "ok"}
		data, err := json.Marshal(response)
		if err != nil {
			http.Error(rw, "encode error", http.StatusInternalServerError)
			return
		}

		if key != "" {
			mac := hmac.New(sha256.New, []byte(key))
			mac.Write(data)
			sig := mac.Sum(nil)
			rw.Header().Set("HashSHA256", hex.EncodeToString(sig))
		}

		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)

			_, err := rw.Write(data)
			if err != nil {
				log.Printf("json write error: %v", err)
			}
		} else {
			rw.Header().Set("Content-Type", "text/html")
			rw.WriteHeader(http.StatusOK)

			_, err := rw.Write([]byte("<html><body><h1>OK</h1></body></html>"))
			if err != nil {
				log.Printf("html write error: %v", err)
			}
		}
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

func UpdateJSONHandler(storage repository.Storage, key string) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var metric models.Metrics

		err := json.NewDecoder(r.Body).Decode(&metric)
		if err != nil {
			http.Error(rw, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		switch metric.MType {
		case "gauge":
			err := storage.SetGauge(metric.ID, repository.Gauge(*metric.Value))
			if err != nil {
				log.Printf("failed to set gauge %s: %v", metric.ID, err)
			}
		case "counter":
			err := storage.SetCounter(metric.ID, repository.Counter(*metric.Delta))
			if err != nil {
				log.Printf("failed to set counter %s: %v", metric.ID, err)
			}
		default:
			http.Error(rw, "unknown type of metric", http.StatusBadRequest)
			return
		}

		response := map[string]string{"status": "ok"}
		data, err := json.Marshal(response)
		if err != nil {
			http.Error(rw, "encode error", http.StatusInternalServerError)
			return
		}

		if key != "" {
			mac := hmac.New(sha256.New, []byte(key))
			mac.Write(data)
			sig := mac.Sum(nil)
			rw.Header().Set("HashSHA256", hex.EncodeToString(sig))
		}

		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)

			_, err = rw.Write(data)
			if err != nil {
				log.Printf("json write error: %v", err)
			}
		} else {
			rw.Header().Set("Content-Type", "text/html")
			rw.WriteHeader(http.StatusOK)

			_, err = rw.Write([]byte("<html><body><h1>OK</h1></body></html>"))
			if err != nil {
				log.Printf("html write error: %v", err)
			}
		}
	}
}

func GetJSONHandler(storage repository.Storage, key string) http.HandlerFunc {
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
				log.Printf("read gauge error: %v", err)
				rw.WriteHeader(http.StatusNotFound)
				return
			}
			metric.Value = new(float64)
			*metric.Value = float64(val)

		case "counter":
			val, err := storage.GetCounter(metric.ID)
			if err != nil {
				log.Printf("read counter error: %v", err)
				rw.WriteHeader(http.StatusNotFound)
				return
			}
			metric.Delta = new(int64)
			*metric.Delta = int64(val)

		default:
			http.Error(rw, "unknown type of metric", http.StatusBadRequest)
			return
		}

		data, err := json.Marshal(metric)
		if err != nil {
			http.Error(rw, "encode error", http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")

		if key != "" {
			mac := hmac.New(sha256.New, []byte(key))
			mac.Write(data)
			sig := mac.Sum(nil)
			rw.Header().Set("HashSHA256", hex.EncodeToString(sig))
		}

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			rw.Header().Set("Content-Encoding", "gzip")
			rw.WriteHeader(http.StatusOK)

			gz := gzip.NewWriter(rw)
			defer gz.Close()

			_, err := gz.Write(data)
			if err != nil {
				log.Printf("response gzip encode error: %v", err)
			}
		} else {
			rw.WriteHeader(http.StatusOK)
			_, err = rw.Write(data)
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
				switch metric.MType {
				case "gauge":
					gaugeCount++
				case "counter":
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
