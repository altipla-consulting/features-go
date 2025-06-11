package features

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/altipla-consulting/env"
	"github.com/altipla-consulting/errors"
	"golang.org/x/sync/singleflight"
)

var isLocal = env.IsLocal()

var client *featuresClient

type featuresClient struct {
	url      string
	flags    []*flagReply
	lastTime time.Time
	mu       sync.RWMutex
	sf       singleflight.Group
}

type flagReply struct {
	Code    string   `json:"code"`
	Tenants []string `json:"tenants"`
	Global  bool     `json:"global"`
}

// Initializes the feature client with the provided server URL and project,
// and starts a background synchronization process.
func Configure(serverURL, project string) error {
	qs := make(url.Values)
	qs.Set("project", project)

	u, err := url.Parse(serverURL)
	if err != nil {
		return errors.Trace(err)
	}
	u.Path += "/eval"
	u.RawQuery = qs.Encode()

	client = &featuresClient{url: u.String()}

	go client.backgroundSync()

	return nil
}

func (c *featuresClient) get() bool {
	c.mu.RLock()
	if time.Since(c.lastTime) < 15*time.Second {
		c.mu.RUnlock()
		return true
	}
	c.mu.RUnlock()

	return false
}

func (c *featuresClient) fetch(ctx context.Context) error {
	if c.get() {
		return nil
	}

	_, err, _ := c.sf.Do("fetch", func() (interface{}, error) {
		var lastErr error
		ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
		defer cancel()

		for i := 0; i < 3; i++ {
			reqCtx, reqCancel := context.WithTimeout(ctx, 3*time.Second)
			req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.url, nil)
			if err != nil {
				reqCancel()
				return nil, err
			}

			reply, err := http.DefaultClient.Do(req)
			reqCancel()
			if err != nil {
				lastErr = err
				if errors.Is(err, context.DeadlineExceeded) {
					select {
					case <-time.After(1 * time.Second):
						continue
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				return nil, err
			}

			body, err := io.ReadAll(reply.Body)
			if err != nil {
				lastErr = err
				continue
			}
			defer reply.Body.Close()

			var flags []*flagReply
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

		return nil, lastErr
	})

	return err
}

func (c *featuresClient) backgroundSync() {
	if isLocal || env.IsCloudRun() {
		return
	}

	ticker := time.NewTicker(13 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.fetch(context.Background()); err != nil {
				slog.Error("Failed to sync background feature flags", slog.String("error", err.Error()))
			}
		}
	}
}

type featuresOption func(*featuresOptions)

type featuresOptions struct {
	tenant string
}

// WithTenant sets the tenant for the flag.
func WithTenant(tenant string) featuresOption {
	return func(o *featuresOptions) {
		o.tenant = tenant
	}
}

// Flag returns true if the flag is enabled with the given options.
func Flag(ctx context.Context, code string, opts ...featuresOption) bool {
	if isLocal {
		return true
	}

	if client == nil {
		panic("Feature flags not configured")
	}

	options := new(featuresOptions)
	for _, opt := range opts {
		opt(options)
	}

	if err := client.fetch(ctx); err != nil {
		return false
	}
	flagsByCode := make(map[string]*flagReply)
	for _, flag := range client.flags {
		flagsByCode[flag.Code] = flag
	}
	if _, ok := flagsByCode[code]; !ok {
		slog.Warn("Feature flag not found", slog.String("code", code))
		return false
	}

	if flagsByCode[code].Global {
		if options.tenant != "" {
			if slices.Contains(flagsByCode[code].Tenants, options.tenant) {
				return true
			}
		} else {
			return true
		}
	}

	return false
}
