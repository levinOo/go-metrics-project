// Package models содержит структуры данных, описывающие основные сущности предметной области.
// Пакет не содержит бизнес-логику и используется для передачи данных между слоями приложения.
package models

// Константы типов метрик
const (
	// Counter представляет метрику-счётчик, значение которой только увеличивается.
	Counter = "counter"

	// Gauge представляет метрику-измеритель, значение которой может изменяться произвольно.
	Gauge = "gauge"
)

// ListMetrics содержит список метрик для пакетной обработки.
type ListMetrics struct {
	// List содержит массив метрик для одновременной отправки или обработки.
	List []Metrics
}

// Metrics представляет отдельную метрику в системе мониторинга.
// Поддерживает два типа метрик: gauge (с полем Value) и counter (с полем Delta).
type Metrics struct {
	// ID содержит уникальное имя метрики.
	ID string `json:"id"`

	// MType определяет тип метрики: "gauge" или "counter".
	MType string `json:"type"`

	// Delta содержит значение для counter-метрик (изменение счётчика).
	// Используется только когда MType = "counter".
	Delta *int64 `json:"delta,omitempty"`

	// Value содержит значение для gauge-метрик (текущее измерение).
	// Используется только когда MType = "gauge".
	Value *float64 `json:"value,omitempty"`

	// Hash содержит HMAC SHA256 подпись метрики для проверки целостности.
	Hash string `json:"hash,omitempty"`
}

// Data представляет событие аудита с информацией об обновлении метрик.
// Используется для логирования операций с метриками.
type Data struct {
	// TS содержит временную метку события в формате Unix timestamp.
	TS int64 `json:"ts"`

	// MetricNames содержит список имён метрик, участвовавших в операции.
	MetricNames []string `json:"metrics"`

	// IP содержит IP-адрес клиента, выполнившего операцию.
	IP string `json:"ip_address"`
}

type DataList struct {
	Events []Data `json:"events"`
}
