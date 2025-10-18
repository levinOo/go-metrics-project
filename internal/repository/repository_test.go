package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/levinOo/go-metrics-project/internal/models"
)

func BenchmarkInsertBatch(b *testing.B) {
	db, mock, err := sqlmock.New()
	if err != nil {
		b.Fatalf("failed to create mock: %v", err)
	}
	defer db.Close()

	storage := NewDBStorage(db)

	val := 42.5
	delta := int64(100)
	metrics := models.ListMetrics{
		List: []models.Metrics{
			{ID: "gauge1", MType: "gauge", Value: &val},
			{ID: "counter1", MType: "counter", Delta: &delta},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mock.ExpectExec(`INSERT INTO metrics`).
			WillReturnResult(sqlmock.NewResult(0, 2))

		err := storage.InsertMetricsBatch(metrics)
		if err != nil {
			b.Fatalf("iteration %d failed: %v", i, err)
		}
	}
}
