package features

import (
	"log/slog"
)

var DefaultClient *featuresClient

// Initializes the feature client with the provided server URL and project,
// and starts a background synchronization process.
func Configure(serverURL, project string, opts ...ConfigureOption) {
	o := new(configureOptions)
	for _, opt := range opts {
		opt(o)
	}
	DefaultClient = newClient(serverURL, project, o)
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

// Flag returns true if the flag is enabled with the given options.
func Flag(code string, opts ...FlagOption) bool {
	// Uninitialized client is considered as disabled.
	if DefaultClient == nil {
		return false
	}

	o := new(flagOptions)
	for _, opt := range opts {
		opt(o)
	}
	return DefaultClient.IsEnabled(code, o.tenant)
}
