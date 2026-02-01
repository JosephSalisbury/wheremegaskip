package app

import (
	"context"
	"time"
)

// Cacher defines the interface for caching skip locations
type Cacher interface {
	Get(ctx context.Context, key string) ([]SkipLocation, error)
	Set(ctx context.Context, key string, data []SkipLocation, ttl time.Duration) error
}
