package web

import (
	"testing"
	"time"
)

func TestTimeoutWithRetentionAddsRunRetention(t *testing.T) {
	timeout := 2 * time.Minute
	got := timeoutWithRetention(timeout)
	want := timeout + runRetention
	if got != want {
		t.Fatalf("timeoutWithRetention(%s)=%s want=%s", timeout, got, want)
	}
}

func TestTimeoutWithRetentionClampsOverflow(t *testing.T) {
	timeout := maxTimerDuration - runRetention + time.Second
	got := timeoutWithRetention(timeout)
	if got != maxTimerDuration {
		t.Fatalf("expected clamp to maxTimerDuration, got %s", got)
	}
}
