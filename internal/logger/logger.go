// Package logger предоставляет утилиты для логирования HTTP-запросов и ответов.
// Включает обертку ResponseWriter для захвата метаданных ответа и создание zap логгеров.
package logger

import (
	"log"
	"net/http"

	"go.uber.org/zap"
)

// ResponseData содержит метаданные HTTP-ответа для логирования.
// Используется совместно с LoggingRW для отслеживания характеристик ответа.
type ResponseData struct {
	// Status содержит HTTP-код ответа (например, 200, 404, 500).
	Status int

	// Size содержит общий размер тела ответа в байтах.
	// Накапливается при множественных вызовах Write.
	Size int
}

// LoggingRW оборачивает стандартный http.ResponseWriter для захвата метрик ответа.
// Перехватывает вызовы Write и WriteHeader для сбора статистики без изменения поведения.
//
// Используется в middleware для логирования размера ответа и статус-кода.
// Совместим с интерфейсом http.ResponseWriter и может использоваться как drop-in замена.
type LoggingRW struct {
	http.ResponseWriter
	// ResponseData указывает на структуру для накопления метаданных ответа.
	ResponseData *ResponseData
}

// Write записывает данные в ответ и обновляет накопленный размер в ResponseData.
// Реализует метод интерфейса http.ResponseWriter.
//
// Каждый вызов увеличивает ResponseData.Size на количество успешно записанных байт.
// Возвращает количество записанных байт и ошибку, если запись не удалась.
func (r *LoggingRW) Write(b []byte) (int, error) {
	size, err := r.ResponseWriter.Write(b)
	r.ResponseData.Size += size
	return size, err
}

// WriteHeader устанавливает HTTP-код ответа и сохраняет его в ResponseData.
// Реализует метод интерфейса http.ResponseWriter.
//
// Должен вызываться до первого вызова Write, иначе будет автоматически установлен код 200.
// Сохраняет код статуса в ResponseData.Status для последующего логирования.
func (r *LoggingRW) WriteHeader(statusCode int) {
	r.ResponseWriter.WriteHeader(statusCode)
	r.ResponseData.Status = statusCode
}

// NewLogger создает и возвращает настроенный zap.SugaredLogger для development окружения.
func NewLogger() *zap.SugaredLogger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()

	sugar := logger.Sugar()

	return sugar
}
