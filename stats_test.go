package features

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeStats struct {
	forceError bool
	last       *statsRequest
}

func (c *fakeStats) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/stats" {
		if c.forceError {
			return nil, fmt.Errorf("forced error")
		}

		c.last = new(statsRequest)
		if err := json.NewDecoder(req.Body).Decode(c.last); err != nil {
			return nil, err
		}

		return &http.Response{StatusCode: http.StatusOK}, nil
	}

	if req.URL.Path == "/eval" {
		return (new(fakeEval)).RoundTrip(req)
	}

	return &http.Response{StatusCode: http.StatusNotFound}, nil
}

func initStats() *fakeStats {
	tr := new(fakeStats)

	slog.SetLogLoggerLevel(slog.LevelDebug)
	DefaultClient = newClient("https://example.com", "foo-project", &configureOptions{
		logger: slog.Default(),
	})
	DefaultClient.local = false
	DefaultClient.client = &http.Client{Transport: tr}

	return tr
}

func TestStats(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr := initStats()
		defer DefaultClient.Close()

		require.True(t, Flag("global-enabled"))

		synctest.Wait()
		DefaultClient.Close()

		require.Equal(t, "foo-project", tr.last.Project)
		require.Len(t, tr.last.Stats, 1)
		require.Equal(t, "global-enabled", tr.last.Stats[0].Flag)
		require.EqualValues(t, 946684800000, tr.last.Stats[0].Bucket)
		require.EqualValues(t, 1, tr.last.Stats[0].EnabledHits)
		require.EqualValues(t, 1, tr.last.Stats[0].TotalHits)
	})
}

func TestStatsEnabledTotalHits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr := initStats()
		defer DefaultClient.Close()

		require.True(t, Flag("tenant-enabled", WithTenant("foo-tenant")))
		require.True(t, Flag("tenant-enabled", WithTenant("foo-tenant")))
		require.False(t, Flag("tenant-enabled", WithTenant("bar-tenant")))
		require.False(t, Flag("tenant-enabled", WithTenant("bar-tenant")))
		require.False(t, Flag("tenant-enabled", WithTenant("bar-tenant")))

		synctest.Wait()
		DefaultClient.Close()

		require.Len(t, tr.last.Stats, 1)
		require.Equal(t, "tenant-enabled", tr.last.Stats[0].Flag)
		require.EqualValues(t, 946684800000, tr.last.Stats[0].Bucket)
		require.EqualValues(t, 2, tr.last.Stats[0].EnabledHits)
		require.EqualValues(t, 5, tr.last.Stats[0].TotalHits)
	})
}

func TestStatsMultipleFlagsMultipleMinutes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr := initStats()
		defer DefaultClient.Close()

		tr.forceError = true

		require.True(t, Flag("global-enabled"))
		time.Sleep(90 * time.Second)
		require.False(t, Flag("global-disabled"))

		tr.forceError = false

		synctest.Wait()
		DefaultClient.Close()

		require.Len(t, tr.last.Stats, 2)

		require.Equal(t, "global-enabled", tr.last.Stats[0].Flag)
		require.EqualValues(t, 946684800000, tr.last.Stats[0].Bucket)
		require.EqualValues(t, 1, tr.last.Stats[0].EnabledHits)
		require.EqualValues(t, 1, tr.last.Stats[0].TotalHits)

		require.Equal(t, "global-disabled", tr.last.Stats[1].Flag)
		require.EqualValues(t, 946684860000, tr.last.Stats[1].Bucket)
		require.EqualValues(t, 0, tr.last.Stats[1].EnabledHits)
		require.EqualValues(t, 1, tr.last.Stats[1].TotalHits)
	})
}
