package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RedisCache implements Cache using Upstash Redis REST API
type RedisCache struct {
	restURL   string
	restToken string
	client    *http.Client
}

// NewRedisCache creates a new Redis cache using Upstash REST API
func NewRedisCache(restURL, restToken string) *RedisCache {
	return &RedisCache{
		restURL:   restURL,
		restToken: restToken,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Get retrieves data from Redis
func (c *RedisCache) Get(ctx context.Context, key string) ([]SkipLocation, error) {
	url := fmt.Sprintf("%s/get/%s", c.restURL, key)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.restToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Result *string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Result == nil {
		return nil, nil // Cache miss
	}

	var locations []SkipLocation
	if err := json.Unmarshal([]byte(*result.Result), &locations); err != nil {
		return nil, fmt.Errorf("unmarshaling locations: %w", err)
	}

	return locations, nil
}

// Set stores data in Redis with the given TTL
func (c *RedisCache) Set(ctx context.Context, key string, data []SkipLocation, ttl time.Duration) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling data: %w", err)
	}

	ttlSeconds := int(ttl.Seconds())
	url := fmt.Sprintf("%s/setex/%s/%d", c.restURL, key, ttlSeconds)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.restToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	return nil
}
