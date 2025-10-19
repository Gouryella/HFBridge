package server

import (
	"context"
	"log"
	"sync"
	"time"

	"hfbridge/internal/logdb"
)

const (
	usageAggregatorInterval = time.Second
	defaultUsageIdleWindow  = 5 * time.Second
)

type usageAggregator struct {
	store *logdb.Store

	mu       sync.Mutex
	sessions map[string]*usageSession

	idleWindow time.Duration
	ticker     *time.Ticker
	stop       chan struct{}
	stopOnce   sync.Once
}

type usageSession struct {
	entry    logdb.Entry
	start    time.Time
	lastEnd  time.Time
	lastSeen time.Time
}

func newUsageAggregator(store *logdb.Store, idleWindow time.Duration) *usageAggregator {
	if store == nil {
		return nil
	}
	if idleWindow <= 0 {
		idleWindow = defaultUsageIdleWindow
	}
	a := &usageAggregator{
		store:      store,
		sessions:   make(map[string]*usageSession),
		idleWindow: idleWindow,
		ticker:     time.NewTicker(usageAggregatorInterval),
		stop:       make(chan struct{}),
	}
	go a.loop()
	return a
}

func (a *usageAggregator) loop() {
	for {
		select {
		case <-a.ticker.C:
			a.flush(false)
		case <-a.stop:
			a.flush(true)
			a.ticker.Stop()
			return
		}
	}
}

func (a *usageAggregator) makeKey(e logdb.Entry) string {
	return e.ClientIP + "|" + e.Repo
}

func (a *usageAggregator) Add(e logdb.Entry) {
	if a == nil || e.Repo == "" || e.Bytes <= 0 {
		return
	}
	now := time.Now()
	if e.Timestamp.IsZero() {
		e.Timestamp = now
	}
	key := a.makeKey(e)
	a.mu.Lock()

	session, ok := a.sessions[key]
	if !ok {
		session = &usageSession{
			entry: logdb.Entry{
				Timestamp:   e.Timestamp,
				ClientIP:    e.ClientIP,
				Repo:        e.Repo,
				RequestPath: e.RequestPath,
				Upstream:    e.Upstream,
				Method:      e.Method,
			},
			start:   e.Timestamp,
			lastEnd: e.Timestamp.Add(e.Duration),
		}
		a.sessions[key] = session
	}

	if e.Timestamp.Before(session.start) || session.start.IsZero() {
		session.start = e.Timestamp
		session.entry.Timestamp = e.Timestamp
	}
	if session.entry.RequestPath == "" {
		session.entry.RequestPath = e.RequestPath
	}
	end := e.Timestamp.Add(e.Duration)
	if end.After(session.lastEnd) {
		session.lastEnd = end
	}
	session.entry.Bytes += e.Bytes
	session.entry.Upstream = e.Upstream
	session.entry.Method = e.Method
	session.lastSeen = now
	a.mu.Unlock()
}

func (a *usageAggregator) flush(force bool) {
	if a == nil {
		return
	}
	now := time.Now()
	pending := a.collect(now, force)
	a.persist(pending)
}

func (a *usageAggregator) collect(now time.Time, force bool) []logdb.Entry {
	a.mu.Lock()
	defer a.mu.Unlock()

	var entries []logdb.Entry
	for key, session := range a.sessions {
		if session == nil || session.entry.Bytes <= 0 {
			delete(a.sessions, key)
			continue
		}
		if !force && now.Sub(session.lastSeen) < a.idleWindow {
			continue
		}

		entry := a.finalize(session)
		entries = append(entries, entry)
		delete(a.sessions, key)
	}
	return entries
}

func (a *usageAggregator) finalize(session *usageSession) logdb.Entry {
	entry := session.entry
	entry.Timestamp = session.start
	duration := session.lastEnd.Sub(session.start)
	if duration < 0 {
		duration = 0
	}
	entry.Duration = duration
	if entry.RequestPath == "" {
		entry.RequestPath = entry.Repo
	}
	return entry
}

func (a *usageAggregator) persist(entries []logdb.Entry) {
	for _, entry := range entries {
		if entry.Bytes <= 0 {
			continue
		}
		if entry.Duration < 0 {
			entry.Duration = 0
		}
		entry.Timestamp = entry.Timestamp.UTC()
		if err := a.store.RecordUsage(context.Background(), entry); err != nil {
			log.Printf("[usage-agg] record failed: %v", err)
		}
	}
}

func (a *usageAggregator) Stop() {
	if a == nil {
		return
	}
	a.stopOnce.Do(func() {
		close(a.stop)
	})
}
