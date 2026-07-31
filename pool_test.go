package features

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPoolReusesClientForSameProject(t *testing.T) {
	pool := NewPool("https://example.com", WithDisableStats(true))
	defer pool.Close()

	a1 := pool.Client("project-a")
	a2 := pool.Client("project-a")
	require.Same(t, a1, a2)
}

func TestPoolCreatesDistinctClientsPerProject(t *testing.T) {
	pool := NewPool("https://example.com", WithDisableStats(true))
	defer pool.Close()

	a := pool.Client("project-a")
	b := pool.Client("project-b")
	require.NotSame(t, a, b)
	require.Equal(t, "project-a", a.project)
	require.Equal(t, "project-b", b.project)
}

func TestPoolFlagUsesProjectClient(t *testing.T) {
	pool := NewPool("https://example.com", WithDisableStats(true))
	defer pool.Close()

	client := pool.Client("project-a")
	client.local = true

	require.True(t, pool.Flag("project-a", "any-flag"))
	require.Same(t, client, pool.Client("project-a"))
}

func TestPoolClientConcurrent(t *testing.T) {
	pool := NewPool("https://example.com", WithDisableStats(true))
	defer pool.Close()

	const n = 50
	clients := make([]*Client, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			clients[i] = pool.Client("shared-project")
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		require.Same(t, clients[0], clients[i])
	}
}

func TestClientFlag(t *testing.T) {
	client := &Client{
		flags: []flagReply{
			{Code: "global-enabled", Enabled: true},
			{
				Code:    "tenant-enabled",
				Enabled: true,
				Tenants: []flagTenant{
					{Code: "foo-tenant", Enabled: true},
				},
			},
		},
		stale:    time.Now().Add(1 * time.Minute),
		accessCh: make(chan struct{}, 100),
		statsCh:  make(chan accessEvent, 100),
		logger:   slog.Default(),
		local:    false,
	}

	require.True(t, client.Flag("global-enabled"))
	require.True(t, client.Flag("tenant-enabled", WithTenant("foo-tenant")))
	require.False(t, client.Flag("tenant-enabled", WithTenant("other")))
	require.False(t, client.Flag("missing"))
}
