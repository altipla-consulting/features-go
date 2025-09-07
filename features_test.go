package features

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func initFlags() {
	DefaultClient = &featuresClient{
		flags: []flagReply{
			{Code: "global-enabled", Enabled: true},
			{Code: "global-disabled", Enabled: false},
			{
				Code:    "tenant-enabled",
				Enabled: true,
				Tenants: []flagTenant{
					{Code: "foo-tenant", Enabled: true},
				},
			},
			{
				Code:    "tenant-disabled",
				Enabled: false,
				Tenants: []flagTenant{
					{Code: "foo-tenant", Enabled: false},
				},
			},
			{
				Code:    "global-disabled-tenant-enabled",
				Enabled: false,
				Tenants: []flagTenant{
					{Code: "foo-tenant", Enabled: true},
				},
			},
		},
		stale:    time.Now().Add(1 * time.Minute),
		accessCh: make(chan struct{}, 100),
		logger:   slog.Default(),
	}
}

func TestPanicClientNotConfigured(t *testing.T) {
	require.PanicsWithValue(t, "call features.Configure() before using features.Flag()", func() {
		Flag("foo-feature")
	})
}

func TestGlobalFlags(t *testing.T) {
	initFlags()
	require.True(t, Flag("global-enabled"))
	require.False(t, Flag("global-disabled"))
	require.False(t, Flag("not-found"))
}

func TestTenantFlags(t *testing.T) {
	initFlags()
	require.True(t, Flag("tenant-enabled", WithTenant("foo-tenant")))
	require.False(t, Flag("tenant-disabled", WithTenant("foo-tenant")))
	require.False(t, Flag("not-found", WithTenant("foo-tenant")))
	require.True(t, Flag("tenant-enabled", WithTenant("foo-tenant")))
	require.False(t, Flag("tenant-disabled", WithTenant("foo-tenant")))
	require.False(t, Flag("tenant-enabled", WithTenant("not-found")))
	require.False(t, Flag("global-disabled-tenant-enabled", WithTenant("foo-tenant")))
}

type fakeTransport struct {
	delay time.Duration

	mu       sync.Mutex
	requests int
}

func (c *fakeTransport) getRequests() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests
}

func (c *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.requests++
	c.mu.Unlock()

	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode([]flagReply{
		{Code: "global-enabled", Enabled: true},
		{Code: "global-disabled", Enabled: false},
		{
			Code:    "tenant-enabled",
			Enabled: true,
			Tenants: []flagTenant{
				{Code: "foo-tenant", Enabled: true},
			},
		},
		{
			Code:    "tenant-disabled",
			Enabled: false,
			Tenants: []flagTenant{
				{Code: "foo-tenant", Enabled: false},
			},
		},
		{
			Code:    "global-disabled-tenant-enabled",
			Enabled: false,
			Tenants: []flagTenant{
				{Code: "foo-tenant", Enabled: true},
			},
		},
	})

	// Simulate the delay of the request.
	time.Sleep(c.delay)
	if req.Context().Err() != nil {
		return nil, req.Context().Err()
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(&buf),
	}, nil
}

func initFetch(delay time.Duration) *fakeEval {
	tr := &fakeEval{delay: delay}

	slog.SetLogLoggerLevel(slog.LevelDebug)
	DefaultClient = newClient("https://example.com", "foo-project", &configureOptions{
		logger:       slog.Default(),
		disableStats: true,
	})
	DefaultClient.local = false
	DefaultClient.client = &http.Client{Transport: tr}

	return tr
}

func TestFetch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		initFetch(0)
		defer DefaultClient.Close()

		require.True(t, Flag("global-enabled"))
	})
}

func TestFetchTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		initFetch(4 * time.Second)
		defer DefaultClient.Close()

		require.False(t, Flag("global-enabled"))
	})
}

func TestFetchTimeoutWithStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr := initFetch(0)
		defer DefaultClient.Close()

		require.True(t, Flag("global-enabled"))

		tr.delay = 4 * time.Second
		require.True(t, Flag("global-enabled"))
	})
}

func TestFetchErrorNotHammering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		initFetch(4 * time.Second)
		defer DefaultClient.Close()

		require.False(t, Flag("global-enabled"))

		require.WithinDuration(t, DefaultClient.stale, time.Now().Add(5*time.Minute), 1*time.Second)
	})
}

func TestFetchSingleflight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr := initFetch(2 * time.Second)
		DefaultClient.staleDuration = 0
		defer DefaultClient.Close()

		go func() {
			require.True(t, Flag("global-enabled"))
		}()
		go func() {
			require.True(t, Flag("global-enabled"))
		}()

		time.Sleep(2 * time.Second)
		synctest.Wait()

		require.Equal(t, 1, tr.getRequests())
	})
}

func TestFetchNotStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr := initFetch(0)
		defer DefaultClient.Close()

		require.True(t, Flag("global-enabled"))
		require.True(t, Flag("global-enabled"))

		synctest.Wait()

		require.Equal(t, 1, tr.getRequests())
	})
}

func TestFetchProgressiveTimers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr := initFetch(0)
		defer DefaultClient.Close()

		require.True(t, Flag("global-enabled"))
		require.Equal(t, 1, tr.getRequests())

		for i := range 20 {
			time.Sleep(15 * time.Second)
			synctest.Wait()
			require.Equal(t, i+2, tr.getRequests())
		}
		for i := range 25 {
			time.Sleep(1 * time.Minute)
			synctest.Wait()
			require.Equal(t, i+22, tr.getRequests())
		}

		time.Sleep(5 * time.Minute)
		synctest.Wait()
		require.Equal(t, 22+25, tr.getRequests())

		time.Sleep(5 * time.Minute)
		synctest.Wait()
		require.Equal(t, 22+25+1, tr.getRequests())
	})
}

func TestFetchMaxFetchIntervalSkipsFollowUpRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr := initFetch(0)
		defer DefaultClient.Close()

		DefaultClient.staleDuration = 0
		require.True(t, Flag("global-enabled"))
		require.Equal(t, 1, tr.getRequests())

		time.Sleep(14 * time.Second)

		DefaultClient.staleDuration = 15 * time.Second
		require.True(t, Flag("global-enabled"))
		require.Equal(t, 2, tr.getRequests())

		time.Sleep(2 * time.Second)
		require.Equal(t, 2, tr.getRequests())
	})
}
