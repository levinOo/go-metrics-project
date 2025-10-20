// Package audit реализует систему аудита операций с метриками.
// Использует паттерн Observer для уведомления различных подписчиков
// о событиях изменения метрик.
package audit

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/levinOo/go-metrics-project/internal/models"
	"github.com/mailru/easyjson"
)

// Observer определяет интерфейс наблюдателя для системы аудита.
// Позволяет регистрировать подписчиков и уведомлять их о событиях.
type Observer interface {
	// RegisterClient добавляет нового подписчика для получения уведомлений.
	RegisterClient(Consumer)

	// RemoveClient удаляет подписчика из списка получателей уведомлений.
	RemoveClient()

	// NotifyClient отправляет уведомление всем зарегистрированным подписчикам.
	NotifyClient()
}

// Consumer определяет интерфейс потребителя событий аудита.
// Реализации этого интерфейса обрабатывают события различными способами
// (запись в файл, отправка по HTTP и т.д.).
type Consumer interface {
	// Update обрабатывает событие аудита с данными об изменении метрик.
	Update(data models.Data)
}

// Auditer координирует отправку событий аудита зарегистрированным подписчикам.
// Реализует паттерн Observer для уведомления о событиях обновления метрик.
type Auditer struct {
	clients []Consumer
	message models.Data
}

// RegisterClient добавляет нового подписчика в список получателей уведомлений.
func (a *Auditer) RegisterClient(o Consumer) {
	a.clients = append(a.clients, o)
}

// RemoveClient удаляет подписчика из списка.
// TODO: Реализовать логику удаления конкретного клиента.
func (a *Auditer) RemoveClient() {
	// логика удаления Client
}

// NotifyClient отправляет текущее сообщение всем зарегистрированным подписчикам.
func (a *Auditer) NotifyClient() {
	for _, client := range a.clients {
		client.Update(a.message)
	}
}

// SetMessage устанавливает сообщение для отправки подписчикам.
func (a *Auditer) SetMessage(data models.Data) {
	a.message = data
}

// FileAuditer записывает события аудита в JSON файл.
// Реализует интерфейс Consumer для обработки событий через файловую систему.
type FileAuditer struct {
	path string
}

// NewFileAuditer создаёт новый экземпляр FileAuditer для записи в указанный файл.
// Параметры:
//
//	path: путь к файлу для записи событий аудита
//	json: JSON-сериализатор для кодирования данных
func NewFileAuditer(path string) *FileAuditer {
	return &FileAuditer{
		path: path,
	}
}

// Update добавляет новое событие аудита в файл.
// Читает существующие события, добавляет новое и перезаписывает файл.
// Если путь пустой, операция пропускается.
func (a *FileAuditer) Update(data models.Data) {
	if a.path == "" {
		return
	}

	var dataList models.DataList
	fileData, err := os.ReadFile(a.path)
	if err != nil {
		log.Printf("failed to read file %s: %v", a.path, err)
		return
	}

	if len(fileData) == 0 {
		log.Printf("file %s is empty", a.path)
		return
	}

	if err := easyjson.Unmarshal(fileData, &dataList); err != nil {
		log.Printf("json.Unmarshal error: %v", err)
		return
	}

	dataList.Events = append(dataList.Events, data)

	jsonData, err := easyjson.Marshal(&dataList)
	if err != nil {
		log.Printf("json.Marshal error: %v", err)
		return
	}

	err = os.WriteFile(a.path, jsonData, 0644)
	if err != nil {
		log.Printf("write file error: %v", err)
		return
	}
}

// URLAuditer отправляет события аудита на внешний HTTP endpoint.
// Реализует интерфейс Consumer для обработки событий через HTTP.
type URLAuditer struct {
	url string
}

// NewURLAuditer создаёт новый экземпляр URLAuditer для отправки на указанный URL.
// Параметры:
//
//	url: HTTP endpoint для отправки событий
//	json: JSON-сериализатор для кодирования данных
func NewURLAuditer(url string) *URLAuditer {
	return &URLAuditer{
		url: url,
	}
}

// Update отправляет событие аудита на настроенный HTTP endpoint методом POST.
// Если URL пустой, операция пропускается.
// Отправляет данные в формате JSON с Content-Type: application/json.
func (a *URLAuditer) Update(data models.Data) {
	if a.url == "" {
		return
	}

	jsonData, err := easyjson.Marshal(data)
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

// NewAuditEvent создаёт и отправляет событие аудита для списка метрик.
// Настраивает подписчиков для файла и URL, собирает информацию о метриках
// и уведомляет всех подписчиков.
//
// Параметры:
//
//	metrics: список метрик для аудита
//	path: путь к файлу аудита (пустая строка для отключения)
//	url: URL для отправки событий (пустая строка для отключения)
//	ip: IP-адрес клиента, выполнившего операцию
//	json: JSON-сериализатор
func NewAuditEvent(metrics models.ListMetrics, path, url, ip string) {
	ts := time.Now().Unix()

	fileAuditer := NewFileAuditer(path)
	urlAuditer := NewURLAuditer(url)

	data := models.Data{
		TS:          ts,
		IP:          ip,
		MetricNames: make([]string, 0, len(metrics.List)),
	}

	auditer := &Auditer{}
	auditer.RegisterClient(fileAuditer)
	auditer.RegisterClient(urlAuditer)

	for _, metric := range metrics.List {
		data.MetricNames = append(data.MetricNames, metric.ID)
	}

	auditer.SetMessage(data)
	auditer.NotifyClient()
}
