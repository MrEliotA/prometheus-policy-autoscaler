package history

import (
    "sync"

    "github.com/MreliotA/prometheus-policy-autoscaler/pkg/policy"
)

// Store is a simple in-memory history store keyed by NamespacedName of the policy.
// For production, you might want to persist history in annotations or an external
// store when you have multiple controller instances.
type Store struct {
    mu   sync.Mutex
    data map[string][]policy.HistorySample
}

// NewStore returns an initialized Store.
func NewStore() *Store {
    return &Store{
        data: make(map[string][]policy.HistorySample),
    }
}

// Get returns a copy of the history for a given key.
func (s *Store) Get(key string) []policy.HistorySample {
    s.mu.Lock()
    defer s.mu.Unlock()

    src := s.data[key]
    out := make([]policy.HistorySample, len(src))
    copy(out, src)
    return out
}

// Append adds a new sample to the history for the given key.
// We keep at most "max" samples; older entries are dropped.
func (s *Store) Append(key string, sample policy.HistorySample, max int) {
    s.mu.Lock()
    defer s.mu.Unlock()

    history := append(s.data[key], sample)
    if len(history) > max {
        history = history[len(history)-max:]
    }
    s.data[key] = history
}
