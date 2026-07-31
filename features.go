package features

import (
	"log/slog"

	"github.com/altipla-consulting/env"
)

var defaultClient *Client

// Configure initializes the default feature client with the provided server URL
// and project, and starts a background synchronization process.
func Configure(serverURL, project string, opts ...ConfigureOption) {
	if defaultClient != nil {
		defaultClient.Close()
	}
	defaultClient = newClient(serverURL, project, configureOptionsFrom(opts))
}

func configureOptionsFrom(opts []ConfigureOption) *configureOptions {
	o := new(configureOptions)
	for _, opt := range opts {
		opt(o)
	}
	return o
}

type ConfigureOption func(*configureOptions)

type configureOptions struct {
	logger       *slog.Logger
	disableStats bool
}

func WithLogger(logger *slog.Logger) ConfigureOption {
	return func(c *configureOptions) {
		c.logger = logger
	}
}

func WithDisableStats(disabled bool) ConfigureOption {
	return func(c *configureOptions) {
		c.disableStats = disabled
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

func applyFlagOptions(opts []FlagOption) *flagOptions {
	o := new(flagOptions)
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Flag returns true if the flag is enabled with the given options.
func Flag(code string, opts ...FlagOption) bool {
	// Uninitialized client is considered as a basic development flag.
	if defaultClient == nil {
		return env.IsLocal()
	}

	return defaultClient.Flag(code, opts...)
}

// Flag returns true if the flag is enabled with the given options.
func (c *Client) Flag(code string, opts ...FlagOption) bool {
	o := applyFlagOptions(opts)
	return c.IsEnabled(code, o.tenant)
}
