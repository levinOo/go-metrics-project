package logger

import (
	"log"
	"net/http"

	"go.uber.org/zap"
)

type ResponseData struct {
	Status int
	Size   int
}

type LoggingRW struct {
	http.ResponseWriter
	ResponseData *ResponseData
}

func (r *LoggingRW) Write(b []byte) (int, error) {
	size, err := r.ResponseWriter.Write(b)
	r.ResponseData.Size += size
	return size, err
}

func (r *LoggingRW) WriteHeader(statusCode int) {
	r.ResponseWriter.WriteHeader(statusCode)
	r.ResponseData.Status = statusCode
}

func NewLogger() *zap.SugaredLogger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()

	sugar := logger.Sugar()

	return sugar
}
