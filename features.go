package features

import (
	"fmt"
	"net/url"
)

var DefaultClient *featuresClient

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
	DefaultClient = newClient(u.String())
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
	if DefaultClient == nil {
		panic("call features.Configure() before using features.Flag()")
	}

	o := new(flagOptions)
	for _, opt := range opts {
		opt(o)
	}
	return DefaultClient.IsEnabled(code, o.tenant)
}
