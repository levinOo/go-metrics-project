// Package handler предоставляет HTTP-обработчики и middleware для сервера метрик.
// Включает роутинг запросов, обработку JSON/text форматов, сжатие данных,
// проверку HMAC-подписей, дешифровку RSA и логирование запросов.
package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rsa"
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
	"github.com/levinOo/go-metrics-project/internal/audit"
	"github.com/levinOo/go-metrics-project/internal/config"
	"github.com/levinOo/go-metrics-project/internal/cryptoutil"
	"github.com/levinOo/go-metrics-project/internal/logger"
	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/levinOo/go-metrics-project/internal/repository"
	"go.uber.org/zap"
)

// NewRouter создает и настраивает HTTP-роутер с использованием chi.
// Регистрирует все обработчики для работы с метриками и применяет middleware.
//
// Зарегистрированные эндпоинты:
//
//	GET  /           - список всех метрик (HTML или text)
//	GET  /ping       - проверка доступности БД
//	POST /updates    - пакетное обновление метрик (JSON)
//	POST /update/    - обновление метрики (JSON)
//	POST /update/{typeMetric}/{metric}/{value} - обновление метрики (URL)
//	POST /value/     - получение значения метрики (JSON)
//	GET  /value/{typeMetric}/{metric} - получение значения метрики (URL)
//
// Middleware применяются в следующем порядке:
//  1. LoggerMiddleware - логирование запросов
//  2. DecryptMiddleware - дешифровка RSA
//  3. HashValidationMiddleware - проверка HMAC
//  4. DecompressMiddleware - декомпрессия gzip
func NewRouter(storage repository.Storage, sugar *zap.SugaredLogger, cfg config.Config) *chi.Mux {
	r := chi.NewRouter()

	r.Use(LoggerMiddleware(sugar))
	r.Use(DecryptMiddleware(cfg.CryptoKeyPath))
	r.Use(HashValidationMiddleware(cfg.Key))
	r.Use(DecompressMiddleware())

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

// LoggerMiddleware создает middleware для логирования HTTP-запросов.
// Записывает URI, метод, длительность, статус и размер ответа.
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

// DecompressMiddleware создает middleware для декомпрессии gzip-сжатых запросов.
// Проверяет заголовок Content-Encoding и распаковывает тело при значении "gzip".
// Возвращает HTTP 400 при ошибках декомпрессии.
func DecompressMiddleware() func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Encoding") != "gzip" {
				log.Printf("DEBUG: No gzip encoding, skipping decompression")
				h.ServeHTTP(rw, r)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("ERROR: Failed to read body for decompression: %v", err)
				http.Error(rw, "read body error", http.StatusBadRequest)
				return
			}
			r.Body.Close()

			log.Printf("DEBUG: Decompressing %d bytes", len(body))

			gr, err := gzip.NewReader(bytes.NewReader(body))
			if err != nil {
				log.Printf("ERROR: Failed to create gzip reader: %v", err)
				http.Error(rw, "decompression error", http.StatusBadRequest)
				return
			}
			defer gr.Close()

			decompressed, err := io.ReadAll(gr)
			if err != nil {
				log.Printf("ERROR: Failed to decompress: %v", err)
				http.Error(rw, "decompression error", http.StatusBadRequest)
				return
			}

			log.Printf("DEBUG: Decompressed successfully: %d bytes -> %d bytes", len(body), len(decompressed))

			r.Body = io.NopCloser(bytes.NewReader(decompressed))
			h.ServeHTTP(rw, r)
		})
	}
}

// DecryptMiddleware создает middleware для дешифровки RSA-зашифрованных запросов.
// Загружает приватный ключ из файла и расшифровывает тело запроса гибридным методом (AES+RSA).
// Пропускает запросы, если приватный ключ не задан или тело пустое.
// Возвращает HTTP 400 при ошибках дешифровки.
func DecryptMiddleware(privateKeyPath string) func(h http.Handler) http.Handler {
	var privateKey *rsa.PrivateKey
	if privateKeyPath != "" {
		var err error
		privateKey, err = cryptoutil.LoadPrivateKey(privateKeyPath)
		if err != nil {
			log.Printf("ERROR: failed to load private key: %v", err)
		} else {
			log.Printf("INFO: Private key loaded successfully")
		}
	}

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("ERROR: failed to read body: %v", err)
				http.Error(rw, "read body error", http.StatusBadRequest)
				return
			}
			r.Body.Close()

			log.Printf("DEBUG: Received %d bytes, privateKey != nil: %v", len(body), privateKey != nil)

			if privateKey != nil && len(body) > 0 {
				decryptedBody, err := cryptoutil.DecryptDataHybrid(privateKey, body)
				if err != nil {
					log.Printf("ERROR: Decryption failed: %v (body length: %d)", err, len(body))
					http.Error(rw, "decryption failed", http.StatusBadRequest)
					return
				}
				log.Printf("DEBUG: Decrypted successfully: %d bytes -> %d bytes", len(body), len(decryptedBody))
				body = decryptedBody
			}

			r.Body = io.NopCloser(bytes.NewReader(body))
			h.ServeHTTP(rw, r)
		})
	}
}

// HashValidationMiddleware создает middleware для проверки HMAC SHA256 подписей.
// Проверяет заголовок HashSHA256 и сравнивает с вычисленной подписью.
// Пропускает запросы без подписи, с подписью "none" или при отсутствии ключа.
// Возвращает HTTP 400 при несовпадении подписей или некорректном формате.
func HashValidationMiddleware(key string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			receivedHash := r.Header.Get("HashSHA256")
			log.Printf("DEBUG Hash: received='%s', key set=%v", receivedHash, key != "")

			if receivedHash == "" || receivedHash == "none" || key == "" {
				log.Printf("DEBUG Hash: Skipping validation")
				h.ServeHTTP(rw, r)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("ERROR Hash: Failed to read body: %v", err)
				http.Error(rw, "read body error", http.StatusBadRequest)
				return
			}
			r.Body.Close()

			log.Printf("DEBUG Hash: Validating hash on %d bytes", len(body))

			sig, err := hex.DecodeString(receivedHash)
			if err != nil {
				log.Printf("ERROR Hash: Bad hash format: %v", err)
				http.Error(rw, "bad hash format", http.StatusBadRequest)
				return
			}

			hash := hmac.New(sha256.New, []byte(key))
			hash.Write(body)
			expectedSig := hash.Sum(nil)

			log.Printf("DEBUG Hash: Expected=%x Received=%x", expectedSig, sig)

			if !hmac.Equal(expectedSig, sig) {
				log.Printf("ERROR Hash: Mismatch!")
				http.Error(rw, "invalid hash", http.StatusBadRequest)
				return
			}

			log.Printf("DEBUG Hash: Validation passed")
			r.Body = io.NopCloser(bytes.NewReader(body))
			h.ServeHTTP(rw, r)
		})
	}
}

// PingHandler возвращает обработчик для проверки доступности базы данных.
// Выполняет ping к хранилищу с таймаутом 2 секунды.
// Возвращает HTTP 200 при успехе, HTTP 500 при недоступности БД.
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

// UpdatesValuesHandler возвращает обработчик для пакетного обновления метрик.
// Принимает массив метрик в JSON и обновляет их одной транзакцией.
// Создает событие аудита и добавляет HMAC-подпись в ответ при наличии ключа.
// Возвращает HTTP 200 при успехе, HTTP 400/500 при ошибках.
func UpdatesValuesHandler(storage repository.Storage, key, path, url string) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("ERROR Handler: Failed to read body: %v", err)
			http.Error(rw, "failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		log.Printf("DEBUG Handler: Read %d bytes", len(body))

		var metrics []models.Metrics
		err = json.Unmarshal(body, &metrics)
		if err != nil {
			log.Printf("ERROR Handler: Unmarshal failed: %v", err)
			http.Error(rw, "invalid JSON format", http.StatusBadRequest)
			return
		}

		log.Printf("DEBUG Handler: Parsed %d metrics", len(metrics))

		listMetrics := models.ListMetrics{List: metrics}

		err = storage.InsertMetricsBatch(listMetrics)
		if err != nil {
			log.Printf("ERROR Handler: InsertMetricsBatch failed: %v", err)
			http.Error(rw, "internal server error", http.StatusInternalServerError)
			return
		}

		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		audit.NewAuditEvent(listMetrics, path, url, ip)

		data := []byte(`{"status":"ok"}`)

		if key != "" {
			mac := hmac.New(sha256.New, []byte(key))
			mac.Write(data)
			sig := mac.Sum(nil)
			rw.Header().Set("HashSHA256", hex.EncodeToString(sig))
		}

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		rw.Write(data)

		log.Printf("DEBUG Handler: Response sent successfully")
	}
}

// UpdateValueHandler возвращает обработчик для обновления метрики через URL параметры.
// Извлекает тип, имя и значение метрики из пути запроса.
// Поддерживает типы "gauge" и "counter".
// Возвращает HTTP 200 при успехе, HTTP 400/404 при ошибках.
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

// UpdateJSONHandler возвращает обработчик для обновления метрики в формате JSON.
// Принимает объект метрики и обновляет её значение.
// Добавляет HMAC-подпись в ответ при наличии ключа.
// Поддерживает content negotiation (JSON/HTML).
// Возвращает HTTP 200 при успехе, HTTP 400 при ошибках.
func UpdateJSONHandler(storage repository.Storage, key string) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, "failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var metric models.Metrics
		err = metric.UnmarshalJSON(body)
		if err != nil {
			http.Error(rw, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

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

		data := []byte(`{"status":"ok"}`)

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

// GetJSONHandler возвращает обработчик для получения значения метрики в JSON.
// Принимает запрос с идентификатором и типом метрики.
// Добавляет HMAC-подпись в заголовок HashSHA256.
// Поддерживает gzip-сжатие ответа.
// Возвращает HTTP 200 при успехе, HTTP 400/404 при ошибках.
func GetJSONHandler(storage repository.Storage, key string) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, "failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var metric models.Metrics
		err = metric.UnmarshalJSON(body)
		if err != nil {
			http.Error(rw, "invalid JSON: "+err.Error(), http.StatusBadRequest)
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

		data, err := metric.MarshalJSON()
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

// GetValueHandler возвращает обработчик для получения значения метрики через URL.
// Извлекает тип и имя метрики из пути.
// Возвращает значение в текстовом формате.
// Возвращает HTTP 200 при успехе, HTTP 400/404 при ошибках.
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

// GetListHandler возвращает обработчик для получения списка всех метрик.
// Форматирует вывод в зависимости от заголовка Accept (HTML или plain text).
// Поддерживает gzip-сжатие ответа.
// Возвращает HTTP 200 при успехе, HTTP 500 при ошибках.
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

			for _, metric := range metrics.List {
				switch metric.MType {
				case "gauge":
					gaugeCount++
				case "counter":
					counterCount++
				}
			}

			if gaugeCount > 0 {
				sb.WriteString("<h2>Gauges</h2><ul>")
				for _, metric := range metrics.List {
					if metric.MType == "gauge" && metric.Value != nil {
						sb.WriteString(fmt.Sprintf("<li>%s: %f</li>", metric.ID, *metric.Value))
					}
				}
				sb.WriteString("</ul>")
			}

			if counterCount > 0 {
				sb.WriteString("<h2>Counters</h2><ul>")
				for _, metric := range metrics.List {
					if metric.MType == "counter" && metric.Delta != nil {
						sb.WriteString(fmt.Sprintf("<li>%s: %d</li>", metric.ID, *metric.Delta))
					}
				}
				sb.WriteString("</ul>")
			}

			sb.WriteString("</body></html>")
		} else {
			for _, metric := range metrics.List {
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
