package neko

import (
	"testing"
	"time"
)

// SetObserverTiming shortens the observer's timeouts for a test.
func SetObserverTiming(t *testing.T, timeout, pingEvery time.Duration) {
	oldTimeout, oldPing := observerTimeout, observerPingEvery
	observerTimeout, observerPingEvery = timeout, pingEvery
	t.Cleanup(func() { observerTimeout, observerPingEvery = oldTimeout, oldPing })
}
