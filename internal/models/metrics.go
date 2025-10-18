package models

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"time"

	jsoniter "github.com/json-iterator/go"
)

const (
	Counter = "counter"
	Gauge   = "gauge"
)

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
	TS          int64    `json:"ts"`
	MetricNames []string `json:"metrics"`
	IP          string   `json:"ip_address"`
}

type Observer interface {
	RegisterClient(Consumer)
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

type FileAuditer struct {
	path string
	json jsoniter.API
}

func NewFileAuditer(path string, json jsoniter.API) *FileAuditer {
	return &FileAuditer{
		path: path,
		json: json,
	}
}

func (a *FileAuditer) Update(data Data) {
	if a.path == "" {
		return
	}

	var events []Data
	fileData, err := os.ReadFile(a.path)
	if err == nil && len(fileData) > 0 {
		if err := a.json.Unmarshal(fileData, &events); err != nil {
			log.Printf("json.Unmarshal error: %v", err)
		}
	}

	events = append(events, data)

	jsonData, err := a.json.MarshalIndent(events, "", "  ")
	if err != nil {
		log.Printf("json.MarshalIndent error: %v", err)
		return
	}

	err = os.WriteFile(a.path, jsonData, 0644)
	if err != nil {
		log.Printf("write file error: %v", err)
	}
}

type URLAuditer struct {
	url  string
	json jsoniter.API
}

func NewURLAuditer(url string, json jsoniter.API) *URLAuditer {
	return &URLAuditer{
		url:  url,
		json: json,
	}
}

func (a *URLAuditer) Update(data Data) {
	if a.url == "" {
		return
	}

	jsonData, err := a.json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("json.marshal error: %v", err)
		return
	}

	resp, err := http.Post(a.url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("HTTP POST request error: %v", err)
		return
	}
	defer resp.Body.Close()
}

func (l *ListMetrics) NewAuditEvent(path, url, ip string, json jsoniter.API) {
	ts := time.Now().Unix()

	fileAuditer := NewFileAuditer(path, json)
	urlAuditer := NewURLAuditer(url, json)
	data := &Data{TS: ts, IP: ip}

	auditer := &Auditer{}
	auditer.RegisterClient(fileAuditer)
	auditer.RegisterClient(urlAuditer)

	for _, name := range l.List {
		data.MetricNames = append(data.MetricNames, name.ID)
	}

	auditer.message = *data
	auditer.NotifyClient()
}
