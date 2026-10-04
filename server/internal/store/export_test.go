package store

import (
	"testing"
	"time"
)

// SetClock replaces the store clock for a test.
func SetClock(t *testing.T, s *Store, now func() time.Time) {
	old := s.now
	s.now = now
	t.Cleanup(func() { s.now = old })
}
