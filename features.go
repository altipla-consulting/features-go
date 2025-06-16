package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/altipla-consulting/env"
	"golang.org/x/sync/singleflight"
)

var isLocal = env.IsLocal()

var client *featuresClient

type featuresClient struct {
	url string

	sf singleflight.Group

	mu       sync.RWMutex // protects flags and lastTime
	flags    []flagReply
	lastTime time.Time
}

type flagReply struct {
	Code    string   `json:"code"`
	Tenants []string `json:"tenants"`
	Global  bool     `json:"global"`
}

// Initializes the feature client with the provided server URL and project,
// and starts a background synchronization process.
func Configure(serverURL, project string) {
	qs := make(url.Values)
	qs.Set("project", project)

	u, err := url.Parse(serverURL)
	if err != nil {
		panic(fmt.Sprintf("cannot parse features url: %s", err.Error()))
	}
	u.Path += "/eval"
	u.RawQuery = qs.Encode()

	client = &featuresClient{url: u.String()}

	go client.backgroundFetch()
}

func (c *featuresClient) get() map[string]flagReply {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if time.Since(c.lastTime) < 15*time.Second {
		return c.flagsMap()
	}

	return nil
}

func (c *featuresClient) flagsMap() map[string]flagReply {
	flagsByCode := make(map[string]flagReply)
	for _, flag := range client.flags {
		flagsByCode[flag.Code] = flag
	}
	return flagsByCode
}

func (c *featuresClient) fetch(ctx context.Context) map[string]flagReply {
	if flags := c.get(); flags != nil {
		return flags
	}

	fn := func() (interface{}, error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
		defer cancel()

		var lastErr error
		for i := 0; i < 3; i++ {
			reqCtx, reqCancel := context.WithTimeout(ctx, 3*time.Second)
			defer reqCancel()
			req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.url, nil)
			if err != nil {
				return c.flagsMap(), err
			}

			reply, err := http.DefaultClient.Do(req)
			if err != nil {
				lastErr = err
				if errors.Is(err, context.DeadlineExceeded) {
					select {
					case <-time.After(1 * time.Second):
						continue
					case <-ctx.Done():
						return c.flagsMap(), ctx.Err()
					}
				}
				return c.flagsMap(), lastErr
			}

			var flags []flagReply
			if err := json.NewDecoder(reply.Body).Decode(&flags); err != nil {
				lastErr = err
				continue
			}
			c.flags = flags
			c.lastTime = time.Now()

			return c.flagsMap(), nil
		}

		return c.flagsMap(), lastErr
	}
	flags, err, _ := c.sf.Do("fetch", fn)
	if err != nil {
		slog.Error("Failed to fetch feature flags", slog.String("error", err.Error()))
	}

	return flags.(map[string]flagReply)
}

func (c *featuresClient) backgroundFetch() {
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

	flags := client.fetch(ctx)
	if flags == nil {
		return false
	}
	if _, ok := flags[code]; !ok {
		slog.Warn("Feature flag not found", slog.String("code", code))
		return false
	}

	if flags[code].Global {
		if options.tenant != "" {
			if slices.Contains(flags[code].Tenants, options.tenant) {
				return true
			}
		} else {
			return true
		}
	}

	return false
}
