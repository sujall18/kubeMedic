package controllers

import "sync"

type IncidentState struct {
	Handling  bool
	Attempts  int
	Handled   bool
	Exhausted bool
}

type IncidentTracker struct {
	mu        sync.Mutex
	incidents map[string]*IncidentState
}

func NewIncidentTracker() *IncidentTracker {
	return &IncidentTracker{
		incidents: make(map[string]*IncidentState),
	}
}

// Start claims an incident.
//
// Once KubeMedic starts handling a workload incident, subsequent Pod events
// for that same workload are ignored until remediation finishes.
func (t *IncidentTracker) Start(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, exists := t.incidents[key]

	if exists {
		if state.Handling || state.Handled || state.Exhausted {
			return false
		}
	}

	if !exists {
		state = &IncidentState{}
		t.incidents[key] = state
	}

	state.Handling = true
	state.Attempts++

	return true
}

// Resolve permanently closes the incident.
func (t *IncidentTracker) Resolve(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, exists := t.incidents[key]; exists {
		state.Handling = false
		state.Handled = true
		state.Exhausted = false
	}
}

// Exhausted permanently stops automatic remediation for this incident.
func (t *IncidentTracker) Exhaust(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, exists := t.incidents[key]; exists {
		state.Handling = false
		state.Handled = false
		state.Exhausted = true
	}
}

// Attempts returns the number of times this incident was claimed.
func (t *IncidentTracker) Attempts(key string) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, exists := t.incidents[key]; exists {
		return state.Attempts
	}

	return 0
}

// Forget removes an incident completely.
// Useful when a workload is deleted/recreated.
func (t *IncidentTracker) Forget(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.incidents, key)
}
