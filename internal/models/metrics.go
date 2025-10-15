package models

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	Counter = "counter"
	Gauge   = "gauge"
)

// NOTE: Не усложняем пример, вводя иерархическую вложенность структур.
// Органичиваясь плоской моделью.
// Delta и Value объявлены через указатели,
// что бы отличать значение "0", от не заданного значения
// и соответственно не кодировать в структуру.
type ListMetrics struct {
	List []Metrics
}

type Metrics struct {
	ID    string   `json:"id"`
	MType string   `json:"type"`
	Delta *int64   `json:"delta,omitempty"`
	Value *float64 `json:"value,omitempty"`
	Hash  string   `json:"hash,omitempty"`
}

type Data struct {
	Ts          int64    `json:"ts"`
	MetricNames []string `json:"metrics"`
	Ip          string   `json:"ip_address"`
}

func (a *Auditer) RegisterClient(o Consumer) {
	a.clients = append(a.clients, o)
}

func (a *Auditer) RemoveClient() {
	// логика удаления Client
}

func (a *Auditer) NotifyClient() {
	for _, client := range a.clients {
		client.Update(a.message)
	}
}

type Observer interface {
	RegisterClient()
	RemoveClient()
	NotifyClient()
}

type Consumer interface {
	Update(data Data)
}

type Auditer struct {
	clients []Consumer
	message Data
}

func (a *FileAuditer) Update(data Data) {
	if a.path == "" {
		return
	}

	var events []Data
	fileData, err := os.ReadFile(a.path)
	if err == nil && len(fileData) > 0 {
		json.Unmarshal(fileData, &events)
	}

	// Добавляем новую запись
	events = append(events, data)

	// Сохраняем как массив
	jsonData, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		log.Printf("json.MarshalIndent error: %v", err)
		return
	}

	err = os.WriteFile(a.path, jsonData, 0644)
	if err != nil {
		log.Printf("write file error: %v", err)
	}
}

type FileAuditer struct {
	path string
}

func (a *URLAuditer) Update(data Data) {
	if a.url == "" {
		return
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("json.marshal error: %v", err)
		return
	}

	resp, err := http.Post(a.url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("HTTP POST request error: %v", err)
		return
	}
	resp.Body.Close()
}

type URLAuditer struct {
	url string
}

func (l *ListMetrics) NewAuditEvent(path, url, ip string) {
	ts := time.Now().Unix()

	fileAuditer := &FileAuditer{path: path}
	urlAuditter := &URLAuditer{url: url}
	data := &Data{Ts: ts, Ip: ip}

	auditer := &Auditer{}
	auditer.RegisterClient(fileAuditer)
	auditer.RegisterClient(urlAuditter)

	for _, name := range l.List {
		data.MetricNames = append(data.MetricNames, name.ID)
	}

	auditer.message = *data
	auditer.NotifyClient()

}
