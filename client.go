package features

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/altipla-consulting/env"
	"golang.org/x/sync/singleflight"
)

type featuresClient struct {
	url    string
	sf     singleflight.Group
	local  bool
	client *http.Client
	logger *slog.Logger

	wg sync.WaitGroup

	mu          sync.RWMutex // protects stale, flags and lastRefresh
	stale       time.Time
	flags       []flagReply
	lastRefresh time.Time

	ticker          *time.Ticker
	lastAccess      time.Time
	refreshInterval time.Duration

	stopCh   chan struct{}
	accessCh chan struct{}

	// constants.
	staleDuration      time.Duration
	staleDurationError time.Duration
	maxFetchInterval   time.Duration
}

func newClient(url string) *featuresClient {
	client := &featuresClient{
		local:              env.IsLocal(),
		client:             http.DefaultClient,
		url:                url,
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		stopCh:             make(chan struct{}),
		accessCh:           make(chan struct{}, 100),
		staleDuration:      1 * time.Minute,
		staleDurationError: 5 * time.Minute,
		refreshInterval:    5 * time.Minute,
		maxFetchInterval:   10 * time.Second,
	}

	client.wg.Add(1)
	go client.backgroundFetch()

	return client
}

type flagReply struct {
	Code    string       `json:"code"`
	Enabled bool         `json:"enabled"`
	Tenants []flagTenant `json:"tenants"`
}

type flagTenant struct {
	Code    string `json:"code"`
	Enabled bool   `json:"enabled"`
}

func (c *featuresClient) backgroundFetch() {
	defer c.wg.Done()

	c.ticker = time.NewTicker(c.refreshInterval)
	defer c.ticker.Stop()

	for {
		select {
		case <-c.ticker.C:
			c.logger.Debug("feature flags: background fetch")

			c.fetch()
			c.adjustInterval()

		case <-c.accessCh:
			c.logger.Debug("feature flags: access registered")

			c.lastAccess = time.Now()
			c.adjustInterval()

		case <-c.stopCh:
			return
		}
	}
}

func (c *featuresClient) adjustInterval() {
	old := c.refreshInterval

	switch sinceAccess := time.Since(c.lastAccess); {
	// First 5 minutes after access, refresh every 15 seconds.
	case sinceAccess < 5*time.Minute:
		c.refreshInterval = 15 * time.Second

	// Next 30 minutes after access, refresh every minute.
	case sinceAccess < 30*time.Minute:
		c.refreshInterval = time.Minute

	// After 30 minutes fallback to once every 5 minutes.
	default:
		c.refreshInterval = 5 * time.Minute
	}

	if c.refreshInterval != old {
		c.logger.Debug("feature flags: adjusting interval", slog.Duration("new", c.refreshInterval), slog.Duration("old", old))
		c.ticker.Reset(c.refreshInterval)
	}
}

func (c *featuresClient) Close() {
	close(c.stopCh)
	c.wg.Wait()
}

func (c *featuresClient) isStale() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stale.IsZero() || time.Since(c.stale) >= 0
}

func (c *featuresClient) fetch() {
	_, _, _ = c.sf.Do("fetch", func() (interface{}, error) {
		c.wg.Add(1)
		defer c.wg.Done()

		c.logger.Debug("feature flags: fetch", slog.Time("stale", c.stale))

		if err := c.safeFetch(); err != nil {
			slog.Warn("feature flags: fetch failed", slog.String("error", err.Error()))

			c.mu.Lock()
			defer c.mu.Unlock()
			c.stale = time.Now().Add(c.staleDurationError)
		}

		return nil, nil
	})
}

func (c *featuresClient) safeFetch() error {
	c.mu.RLock()
	lastFetch := c.lastRefresh
	c.mu.RUnlock()
	if !lastFetch.IsZero() && time.Since(lastFetch) < c.maxFetchInterval {
		c.logger.Debug("feature flags: skip fetch", slog.Time("lastFetch", lastFetch))
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("cannot create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot fetch: %w", err)
	}
	defer resp.Body.Close()

	var fetched []flagReply
	if err := json.NewDecoder(resp.Body).Decode(&fetched); err != nil {
		return fmt.Errorf("cannot decode response: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.flags = fetched
	c.stale = time.Now().Add(c.staleDuration)
	c.lastRefresh = time.Now()

	return nil
}

func (c *featuresClient) IsEnabled(flag, tenant string) bool {
	if c.local {
		return true
	}

	if c.isStale() {
		c.fetch()
	}
	c.accessCh <- struct{}{}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, f := range c.flags {
		if f.Code != flag {
			continue
		}

		// Global flags always depend on the enabled state of the flag.
		if len(f.Tenants) == 0 {
			return f.Enabled
		}

		// Disabled flags always return false for each tenant too.
		if !f.Enabled {
			return false
		}

		// Search for the specific tenant in the list. If we requested an empty one it won't match anyway
		// and return false.
		for _, t := range f.Tenants {
			if t.Code == tenant {
				return t.Enabled
			}
		}

		return false
	}

	return false
}
