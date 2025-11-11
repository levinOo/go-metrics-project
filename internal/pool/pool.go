// Package pool предоставляет обобщённый пул объектов T, ограниченных Reset().
// Пример использования:
//
//	cfgPool := pool.New[*Config](func() *Config { return &Config{} })
//	cfg := cfgPool.Get()
//	// использовать cfg
//	cfgPool.Put(cfg)
package pool

import (
	"sync"
)

// Resettable ограничивает тип тем, у кого есть метод Reset()
type Resettable interface {
	Reset()
}

// Pool хранит объекты типа T, ограниченных Resettable.
// T обычно является указателем на структуру, например *Config.
type Pool[T Resettable] struct {
	mu      sync.Mutex
	items   []T
	Factory func() T
}

// New создаёт новый Pool[T]. Фабрика должна возвращать новый экземпляр T.
func New[T Resettable](factory func() T) *Pool[T] {
	return &Pool[T]{Factory: factory}
}

// Get возвращает объект из пула. Если пула нет или он пуст, создаёт новый через фабрику.
func (p *Pool[T]) Get() T {
	p.mu.Lock()
	n := len(p.items)
	if n > 0 {
		v := p.items[n-1]
		p.items = p.items[:n-1]
		p.mu.Unlock()
		return v
	}
	p.mu.Unlock()

	if p.Factory != nil {
		return p.Factory()
	}
	var zero T
	return zero
}

// Put возвращает объект обратно в пул после вызова Reset()
func (p *Pool[T]) Put(v T) {
	v.Reset()

	p.mu.Lock()
	p.items = append(p.items, v)
	p.mu.Unlock()
}
