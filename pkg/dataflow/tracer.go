package dataflow

import (
	"sync"
	"time"
)

type TraceEvent struct {
	FuncID    string    `json:"func_id"`
	VarName   string    `json:"var_name"`
	Value     string    `json:"value"`
	File      string    `json:"file"`
	Line      int       `json:"line"`
	Goroutine uint64    `json:"goroutine"`
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
}

type Tracer struct {
	mu       sync.Mutex
	events   []TraceEvent
	capacity int
	pos      int
	count    int
}

func NewTracer(capacity int) *Tracer {
	if capacity <= 0 {
		capacity = 10000
	}
	return &Tracer{
		events:   make([]TraceEvent, capacity),
		capacity: capacity,
	}
}

func (t *Tracer) Record(event TraceEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.events[t.pos] = event
	t.pos = (t.pos + 1) % t.capacity
	t.count++
}

func (t *Tracer) GetEvents(funcID string) []TraceEvent {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.count == 0 {
		return nil
	}

	if funcID == "" {
		result := make([]TraceEvent, 0, min(t.count, t.capacity))
		if t.count < t.capacity {
			result = append(result, t.events[:t.count]...)
		} else {
			result = append(result, t.events[t.pos:]...)
			result = append(result, t.events[:t.pos]...)
		}
		return result
	}

	var result []TraceEvent
	n := min(t.count, t.capacity)
	for i := 0; i < n; i++ {
		idx := (t.pos - n + i + t.capacity) % t.capacity
		if t.events[idx].FuncID == funcID {
			result = append(result, t.events[idx])
		}
	}
	return result
}

func (t *Tracer) GetEventsByVariable(varName string) []TraceEvent {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.count == 0 || varName == "" {
		return nil
	}

	var result []TraceEvent
	n := min(t.count, t.capacity)
	for i := 0; i < n; i++ {
		idx := (t.pos - n + i + t.capacity) % t.capacity
		if t.events[idx].VarName == varName {
			result = append(result, t.events[idx])
		}
	}
	return result
}

func (t *Tracer) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.pos = 0
	t.count = 0
}

func (t *Tracer) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return min(t.count, t.capacity)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
