package config

import (
	"context"
	"time"
)

// QueryContext applies the user-configured deadline to all database operations,
// including connection checks, list/search queries, lookup, reads, and writes.
func (c Config) QueryContext() (context.Context, context.CancelFunc) {
	seconds := c.ConnectionTimeoutSeconds
	if seconds <= 0 {
		seconds = 30 // Config literals created without LoadConfig
	}
	return context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
}
