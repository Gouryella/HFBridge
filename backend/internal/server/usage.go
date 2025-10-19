package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"hfbridge/internal/logdb"
)

type usageResponse struct {
	TotalCount    int64   `json:"total_count"`
	TotalBytes    int64   `json:"total_bytes"`
	CurrentBPS    float64 `json:"current_bps"`
	WindowSeconds int     `json:"window_seconds"`
}

func usageHandler(store *logdb.Store, stats *statsCollector) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		snapshot := statsSnapshot{}
		if stats != nil {
			snapshot = stats.Snapshot()
		}

		resp := usageResponse{
			TotalCount:    snapshot.TotalCount,
			TotalBytes:    snapshot.TotalBytes,
			CurrentBPS:    snapshot.CurrentBPS,
			WindowSeconds: snapshot.WindowSeconds,
		}

		if store != nil {
			summary, err := store.UsageSummary(ctx)
			if err != nil {
				log.Printf("[usage-log] query failed: %v", err)
				c.String(http.StatusInternalServerError, "internal error")
				return
			}
			if resp.TotalCount < summary.TotalCount {
				resp.TotalCount = summary.TotalCount
			}
			if resp.TotalBytes == 0 || resp.TotalBytes < summary.TotalBytes {
				resp.TotalBytes = summary.TotalBytes
			}
		} else if resp.WindowSeconds == 0 {
			resp.WindowSeconds = int(defaultStatsWindow / time.Second)
		}

		c.JSON(http.StatusOK, resp)
	}
}

func usageStreamHandler(stats *statsCollector, store *logdb.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if stats == nil {
			c.String(http.StatusServiceUnavailable, "stats unavailable")
			return
		}

		w := c.Writer
		r := c.Request

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			c.String(http.StatusInternalServerError, "stream unsupported")
			return
		}

		ctx := r.Context()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		type streamPayload struct {
			Timestamp     time.Time `json:"timestamp"`
			TotalCount    int64     `json:"total_count"`
			TotalBytes    int64     `json:"total_bytes"`
			CurrentBPS    float64   `json:"current_bps"`
			WindowSeconds int       `json:"window_seconds"`
		}

		var (
			lastSummary time.Time
			summary     logdb.UsageSummary
		)

		send := func() bool {
			snapshot := stats.Snapshot()
			if store != nil && (summary.TotalCount == 0 || time.Since(lastSummary) >= 5*time.Second) {
				if ctx.Err() != nil {
					return false
				}
				s, err := store.UsageSummary(ctx)
				if err != nil {
					log.Printf("[usage-stream] summary query failed: %v", err)
				} else {
					summary = s
					lastSummary = time.Now()
				}
			}

			payload := streamPayload{
				Timestamp:     time.Now().UTC(),
				TotalCount:    snapshot.TotalCount,
				TotalBytes:    snapshot.TotalBytes,
				CurrentBPS:    snapshot.CurrentBPS,
				WindowSeconds: snapshot.WindowSeconds,
			}
			if store != nil {
				if payload.TotalCount < summary.TotalCount {
					payload.TotalCount = summary.TotalCount
				}
				if payload.TotalBytes == 0 || payload.TotalBytes < summary.TotalBytes {
					payload.TotalBytes = summary.TotalBytes
				}
				if payload.WindowSeconds == 0 {
					payload.WindowSeconds = int(defaultStatsWindow / time.Second)
				}
			}
			data, err := json.Marshal(payload)
			if err != nil {
				log.Printf("[usage-stream] marshal failed: %v", err)
				return false
			}

			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				log.Printf("[usage-stream] write failed: %v", err)
				return false
			}
			flusher.Flush()
			return true
		}

		if !send() {
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !send() {
					return
				}
			}
		}
	}
}
