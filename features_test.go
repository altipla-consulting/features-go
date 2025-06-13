package features

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func initTestbed() {
	isLocal = false
	client = &featuresClient{
		flags: []flagReply{
			{
				Code:    "feature-1",
				Tenants: []string{"tenant-1"},
				Global:  true,
			},
			{
				Code:    "feature-2",
				Tenants: []string{"tenant-1"},
				Global:  false,
			},
		},
		lastTime: time.Now(),
	}
}

func TestPanicClientNotConfigured(t *testing.T) {
	initTestbed()
	client = nil
	require.PanicsWithValue(t, "Feature flags not configured", func() {
		Flag(context.Background(), "feature-1")
	})
}

func TestTrueFlag(t *testing.T) {
	initTestbed()
	require.True(t, Flag(context.Background(), "feature-1"))
}

func TestTrueFlagWithTenant(t *testing.T) {
	initTestbed()
	require.True(t, Flag(context.Background(), "feature-1", WithTenant("tenant-1")))
}

func TestFalseFlag(t *testing.T) {
	initTestbed()
	require.False(t, Flag(context.Background(), "feature-2"))
}

func TestFalseFlagWithTenant(t *testing.T) {
	initTestbed()
	require.False(t, Flag(context.Background(), "feature-2", WithTenant("tenant-1")))
}

func TestFalseFlagWithFalseTenant(t *testing.T) {
	initTestbed()
	require.False(t, Flag(context.Background(), "feature-1", WithTenant("tenant-3")))
}

func TestInternalServerError(t *testing.T) {
	initTestbed()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client.url = server.URL
	client.lastTime = time.Now().Add(-1 * time.Minute)

	require.True(t, Flag(context.Background(), "feature-1"))
}

func TestInternalServerErrorFlagsNil(t *testing.T) {
	initTestbed()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client.url = server.URL
	client.flags = nil

	require.False(t, Flag(context.Background(), "feature-1"))
}

func TestTimeout(t *testing.T) {
	initTestbed()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer server.Close()
	client.url = server.URL
	client.lastTime = time.Now().Add(-1 * time.Minute)

	require.True(t, Flag(context.Background(), "feature-1"))
}

func TestTimeoutFlagsNil(t *testing.T) {
	initTestbed()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer server.Close()
	client.url = server.URL
	client.flags = nil

	require.False(t, Flag(context.Background(), "feature-1"))
}
