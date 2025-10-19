package server

import (
	"sync"
	"time"
)

const defaultStatsWindow = time.Second

type statsCollector struct {
	window time.Duration

	mu         sync.Mutex
	totalBytes int64
	totalCount int64
	buckets    map[int64]int64
	lastClean  int64
}

type statsSnapshot struct {
	TotalCount    int64
	TotalBytes    int64
	CurrentBPS    float64
	WindowSeconds int
}

func newStatsCollector(window time.Duration, initialBytes, initialCount int64) *statsCollector {
	if window <= 0 {
		window = defaultStatsWindow
	}
	if window < time.Second {
		window = time.Second
	}
	return &statsCollector{
		window:     window,
		totalBytes: initialBytes,
		totalCount: initialCount,
		buckets:    make(map[int64]int64),
	}
}

func (s *statsCollector) Record(bytes int64) {
	if s == nil || bytes <= 0 {
		return
	}
	now := time.Now()
	sec := now.Unix()

	s.mu.Lock()
	s.totalBytes += bytes
	s.buckets[sec] += bytes
	if sec != s.lastClean {
		s.cleanupLocked(now)
		s.lastClean = sec
	}
	s.mu.Unlock()
}

func (s *statsCollector) IncrementRequests(delta int64) {
	if s == nil || delta == 0 {
		return
	}
	s.mu.Lock()
	s.totalCount += delta
	s.mu.Unlock()
}

func (s *statsCollector) Snapshot() statsSnapshot {
	if s == nil {
		return statsSnapshot{}
	}
	now := time.Now()
	sec := now.Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked(now)
	s.lastClean = sec

	windowBytes := int64(0)
	cutoff := now.Add(-s.window).Unix()
	earliestTs := sec
	for ts, bytes := range s.buckets {
		if ts >= cutoff && ts <= sec {
			windowBytes += bytes
			if ts < earliestTs {
				earliestTs = ts
			}
		}
	}
	windowSeconds := int(s.window / time.Second)
	if windowSeconds <= 0 {
		windowSeconds = 1
	}
	usedWindow := windowSeconds
	if windowBytes > 0 {
		usedWindow = int(sec-earliestTs) + 1
		if usedWindow < 1 {
			usedWindow = 1
		}
		if usedWindow > windowSeconds {
			usedWindow = windowSeconds
		}
	}

	return statsSnapshot{
		TotalCount:    s.totalCount,
		TotalBytes:    s.totalBytes,
		CurrentBPS:    float64(windowBytes) / float64(usedWindow),
		WindowSeconds: windowSeconds,
	}
}

func (s *statsCollector) cleanupLocked(now time.Time) {
	if len(s.buckets) == 0 {
		return
	}
	cutoff := now.Add(-s.window).Unix()
	for ts := range s.buckets {
		if ts < cutoff {
			delete(s.buckets, ts)
		}
	}
}
