package features

import (
	"context"
	"encoding/json"
	"fmt"
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
		panic(fmt.Sprintf("cannot parse features url: %s", err.Error()))
	}
	u.Path += "/eval"
	u.RawQuery = qs.Encode()

	client = &featuresClient{url: u.String()}

	go client.backgroundSync()

	return nil
}

func (c *featuresClient) get() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if time.Since(c.lastTime) < 15*time.Second {
		return true
	}

	return false
}

func (c *featuresClient) fetch(ctx context.Context) bool {
	if c.get() {
		return true
	}

	_, err, _ := c.sf.Do("fetch", func() (interface{}, error) {
		var lastErr error
		ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
		defer cancel()

		for i := 0; i < 3; i++ {
			reqCtx, reqCancel := context.WithTimeout(ctx, 3*time.Second)
			defer reqCancel()
			req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.url, nil)
			if err != nil {
				return nil, err
			}

			reply, err := http.DefaultClient.Do(req)
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

			var flags []*flagReply
			if err := json.NewDecoder(reply.Body).Decode(&flags); err != nil {
				lastErr = err
				continue
			}

			c.mu.Lock()
			defer c.mu.Unlock()
			c.flags = flags
			c.lastTime = time.Now()

			return nil, nil
		}

		return nil, lastErr
	})

	return err == nil
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
			_ = c.fetch(context.Background())
		}
	}
}

type FlagOption func(*flagOptions)

type flagOptions struct {
	tenant string
}

// WithTenant sets the tenant for the flag.
func WithTenant(tenant string) FlagOption {
	return func(o *flagOptions) {
		o.tenant = tenant
	}
}

// Flag returns true if the flag is enabled with the given options.
func Flag(ctx context.Context, code string, opts ...FlagOption) bool {
	if isLocal {
		return true
	}

	if client == nil {
		panic("Feature flags not configured")
	}

	options := new(flagOptions)
	for _, opt := range opts {
		opt(options)
	}

	if !client.fetch(ctx) {
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
