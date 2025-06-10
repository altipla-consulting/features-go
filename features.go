package features

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/altipla-consulting/env"
	"github.com/altipla-consulting/errors"
	"golang.org/x/sync/singleflight"
)

var isLocal = env.IsLocal()

var client *features

type features struct {
	url      string
	flags    []*flag
	lastTime time.Time
	mu       sync.RWMutex
	sf       singleflight.Group
}

type flag struct {
	Code    string   `json:"code"`
	Tenants []string `json:"tenants"`
	Global  bool     `json:"global"`
}

// Configure the features.
func Configure(serverURL, project string) error {
	qs := make(url.Values)
	qs.Set("project", project)

	u, err := url.Parse(serverURL)
	if err != nil {
		return errors.Trace(err)
	}
	u.Path += "/eval"
	u.RawQuery = qs.Encode()

	client = &features{url: u.String()}

	go client.backgroundSync()

	return nil
}

func (c *features) getFlags(ctx context.Context) error {
	c.mu.RLock()
	if time.Since(c.lastTime) < 15*time.Second && c.flags != nil {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	_, err, _ := c.sf.Do("getFlags", func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
		defer cancel()

		var lastErr error
		for i := 0; i < 3; i++ {
			tryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(tryCtx, http.MethodGet, c.url, nil)
			if err != nil {
				return nil, errors.Trace(err)
			}

			reply, err := http.DefaultClient.Do(req)
			if err != nil {
				lastErr = err
				select {
				case <-time.After(1 * time.Second):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}

			body, err := io.ReadAll(reply.Body)
			if err != nil {
				lastErr = err
				continue
			}
			defer reply.Body.Close()

			var flags []*flag
			if err := json.Unmarshal(body, &flags); err != nil {
				lastErr = err
				continue
			}

			c.mu.Lock()
			c.flags = flags
			c.lastTime = time.Now()
			c.mu.Unlock()

			return nil, nil
		}

		return nil, errors.Trace(lastErr)
	})

	return err
}

func (c *features) backgroundSync() {
	if env.IsLocal() || env.IsCloudRun() {
		return
	}

	ticker := time.NewTicker(13 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.getFlags(context.Background()); err != nil {
				slog.Error("Failed to sync background feature flags", slog.String("error", err.Error()))
			}
		}
	}
}

type option func(*options)

type options struct {
	tenant string
}

// WithTenant sets the tenant for the flag.
func WithTenant(tenant string) option {
	return func(o *options) {
		o.tenant = tenant
	}
}

// Flag returns true if the flag is enabled with the given options.
func Flag(ctx context.Context, code string, opts ...option) bool {
	if isLocal {
		return true
	}

	if client == nil {
		slog.Error("Feature flags not configured")
		return false
	}

	options := new(options)
	for _, opt := range opts {
		opt(options)
	}

	if err := client.getFlags(ctx); err != nil {
		return false
	}
	flagsByCode := make(map[string]*flag)
	for _, flag := range client.flags {
		flagsByCode[flag.Code] = flag
	}
	if _, ok := flagsByCode[code]; !ok {
		slog.Warn("Feature flag not found", slog.String("code", code))
		return false
	}

	if flagsByCode[code].Global {
		if options.tenant != "" {
			for _, tenant := range flagsByCode[code].Tenants {
				if tenant == options.tenant {
					return true
				}
			}
		} else {
			return true
		}
	}

	return false
}
