package core

import (
    "context"
    "testing"
    "time"
)

// Query deadlines must follow config.json rather than the historical
// hard-coded 12-second timeout. No real DB is required to test this.
func TestConfiguredSQLQueryDeadline(t *testing.T) {
    cfg := DefaultConfig()
    for _, seconds := range []int{1, 2, 30, 120, 3600} {
        cfg.ConnectionTimeoutSeconds = seconds
        start := time.Now()
        ctx, cancel := cfg.QueryContext()
        deadline, ok := ctx.Deadline()
        if !ok {
            cancel()
            t.Fatalf("%d sec context has no deadline", seconds)
        }
        duration := deadline.Sub(start)
        expected := time.Duration(seconds)*time.Second
        if duration < expected-200*time.Millisecond || duration > expected+200*time.Millisecond {
            cancel()
            t.Fatalf("%d sec got deadline duration %s, want %s", seconds, duration, expected)
        }
        if ctx.Err()!=nil {
            cancel()
            t.Fatal("context should not already be canceled")
        }
        cancel()
        if ctx.Err()!=context.Canceled {
            t.Fatalf("context cancellation failed: %v",ctx.Err())
        }
    }
}
