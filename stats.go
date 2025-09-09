package features

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type accessEvent struct {
	flag    string
	enabled bool
}

func (c *featuresClient) trackAccess(flag string, enabled bool) {
	select {
	case c.statsCh <- accessEvent{flag: flag, enabled: enabled}:
	default:
		c.logger.Debug("feature flags: stats access channel full, dropping event", slog.String("flag", flag))
	}
}

func (c *featuresClient) backgroundStats() {
	slog.Info("feature flags: background stats collector enabled")

	defer c.wg.Done()

	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if err := c.sendStats(c.ctx); err != nil {
				c.logger.Error("feature flags: failed to send stats", slog.String("error", err.Error()))

				// Cleanup stats older than 20 hours.
				cutoff := time.Now().Add(-20 * time.Hour).UnixMilli()
				for flag, flagStats := range c.stats {
					for bucket := range flagStats.buckets {
						if bucket < cutoff {
							delete(flagStats.buckets, bucket)
						}
					}
					if len(flagStats.buckets) == 0 {
						delete(c.stats, flag)
					}
				}
			}

		case event := <-c.statsCh:
			stats, ok := c.stats[event.flag]
			if !ok {
				stats = &flagStats{
					buckets: make(map[int64]*bucketStats),
				}
				c.stats[event.flag] = stats
			}

			key := time.Now().Truncate(time.Minute).UnixMilli()
			bucket, ok := stats.buckets[key]
			if !ok {
				bucket = new(bucketStats)
				stats.buckets[key] = bucket
			}

			bucket.totalHits++
			if event.enabled {
				bucket.enabledHits++
			}

		case <-c.ctx.Done():
			if err := c.sendStats(context.Background()); err != nil {
				c.logger.Error("feature flags: failed to send stats on context done", slog.String("error", err.Error()))
			}
			return
		}
	}
}

type flagStats struct {
	buckets map[int64]*bucketStats
}

type bucketStats struct {
	enabledHits int64
	totalHits   int64
}

func (c *featuresClient) sendStats(ctx context.Context) error {
	if c.local {
		return nil
	}

	if len(c.stats) == 0 {
		return nil
	}

	c.logger.Debug("feature flags: sending stats")

	var stats []statEntry
	for flag, flagStats := range c.stats {
		for bucket, bucketStats := range flagStats.buckets {
			stats = append(stats, statEntry{
				Bucket:      bucket,
				Flag:        flag,
				EnabledHits: bucketStats.enabledHits,
				TotalHits:   bucketStats.totalHits,
			})
		}
	}

	var buf bytes.Buffer
	in := statsRequest{
		Project: c.project,
		Stats:   stats,
	}
	if err := json.NewEncoder(&buf).Encode(in); err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.statsURL, &buf)
	if err != nil {
		return fmt.Errorf("cannot create stats request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot send stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected stats status code %d", resp.StatusCode)
	}

	c.stats = make(map[string]*flagStats)

	return nil
}
