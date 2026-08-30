package controllers

import (
	"sync"
)

type IncidentTracker struct {
	mu        sync.Mutex
	incidents map[string]bool
}

func NewIncidentTracker() *IncidentTracker {
	return &IncidentTracker{
		incidents: make(map[string]bool),
	}
}

func (t *IncidentTracker) IsNew(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.incidents[key] {
		return false
	}

	t.incidents[key] = true

	return true
}

func (t *IncidentTracker) Resolve(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.incidents, key)
}
