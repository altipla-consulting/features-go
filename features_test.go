package features

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func InitTestbed() {
	client = &features{
		tests: true,
		flags: []*flag{
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

func TestFalseClientNotConfigured(t *testing.T) {
	require.False(t, Flag(context.Background(), "feature-1"))
}

func TestTrueFlag(t *testing.T) {
	InitTestbed()
	require.True(t, Flag(context.Background(), "feature-1"))
}

func TestTrueFlagWithTenant(t *testing.T) {
	InitTestbed()
	require.True(t, Flag(context.Background(), "feature-1", WithTenant("tenant-1")))
}

func TestFalseFlag(t *testing.T) {
	InitTestbed()
	require.False(t, Flag(context.Background(), "feature-2"))
}

func TestFalseFlagWithTenant(t *testing.T) {
	InitTestbed()
	require.False(t, Flag(context.Background(), "feature-2", WithTenant("tenant-1")))
}

func TestFalseFlagWithFalseTenant(t *testing.T) {
	InitTestbed()
	require.False(t, Flag(context.Background(), "feature-1", WithTenant("tenant-3")))
}
