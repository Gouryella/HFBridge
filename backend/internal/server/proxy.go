package server

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"hfbridge/internal/config"
	"hfbridge/internal/logdb"
)

type proxyHandler struct {
	cfg       config.Config
	transport *http.Transport
	store     *logdb.Store
	stats     *statsCollector
	usageAgg  *usageAggregator
}

type ctxKeyProxyOrigin struct{}

func newProxyHandler(cfg config.Config, store *logdb.Store, stats *statsCollector) *proxyHandler {
	return &proxyHandler{
		cfg:       cfg,
		transport: newTransport(),
		store:     store,
		stats:     stats,
		usageAgg:  newUsageAggregator(store, 0),
	}
}

func (p *proxyHandler) handleReverseProxy(w http.ResponseWriter, r *http.Request) {
	defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					return
				}
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				return
			}
			panic(rec)
		}
	}()

	target, err := p.resolveUpstreamURL(r.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	repo := extractRepoFromPath(target.Path)
	requestPath := requestPathWithQuery(target.Path, target.RawQuery)
	upstream := target.Scheme + "://" + target.Host

	var stats *statsCollector
	if repo != "" {
		stats = p.stats
	}
	writer := newCountingResponseWriter(w, stats)
	start := time.Now()

	if origin := p.effectiveProxyOrigin(r); origin != "" {
		ctx := context.WithValue(r.Context(), ctxKeyProxyOrigin{}, origin)
		r = r.WithContext(ctx)
	}

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = target.Path
			req.URL.RawPath = target.RawPath
			req.URL.RawQuery = target.RawQuery
			req.Host = target.Host
			p.withUpstreamAuth(req)
			req.Header.Set("Accept-Encoding", "gzip")
		},
		Transport:      p.transport,
		ModifyResponse: p.modifyResponse,
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			log.Printf("[upstream] %s %s: %v", req.Method, req.URL.String(), err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}

	rp.ServeHTTP(writer, r)

	status := writer.Status()
	bytesWritten := writer.BytesWritten()
	if repo != "" && status < http.StatusBadRequest && bytesWritten > 0 {
		entry := logdb.Entry{
			Timestamp:   start,
			ClientIP:    clientIP(r),
			Repo:        repo,
			RequestPath: requestPath,
			Upstream:    upstream,
			Method:      r.Method,
			Bytes:       bytesWritten,
			Duration:    time.Since(start),
		}
		p.logUsage(entry, status)
		p.recordUsage(entry)
		if p.stats != nil {
			p.stats.IncrementRequests(1)
		}
	}
}

func (p *proxyHandler) handleLFSContent(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	if raw == "" {
		http.Error(w, "missing u", http.StatusBadRequest)
		return
	}

	target, err := url.Parse(raw)
	if err != nil || target.Scheme == "" || target.Host == "" {
		http.Error(w, "bad u", http.StatusBadRequest)
		return
	}
	if !p.hostAllowed(target.Host) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}

	requestPath := requestPathWithQuery(target.Path, target.RawQuery)
	upstream := target.Scheme + "://" + target.Host

	var stats *statsCollector
	if repo != "" {
		stats = p.stats
	}
	writer := newCountingResponseWriter(w, stats)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), nil)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	copyHeadersPassThrough(req.Header, r.Header)
	p.withUpstreamAuth(req)
	req.Header.Set("Accept-Encoding", "gzip")

	start := time.Now()
	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHeadersPassThrough(writer.Header(), resp.Header)
	writer.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(writer, resp.Body)

	status := writer.Status()
	bytesWritten := writer.BytesWritten()
	if repo != "" && status < http.StatusBadRequest && bytesWritten > 0 {
		entry := logdb.Entry{
			Timestamp:   start,
			ClientIP:    clientIP(r),
			Repo:        repo,
			RequestPath: requestPath,
			Upstream:    upstream,
			Method:      r.Method,
			Bytes:       bytesWritten,
			Duration:    time.Since(start),
		}
		p.logUsage(entry, status)
		p.recordUsage(entry)
		if p.stats != nil {
			p.stats.IncrementRequests(1)
		}
	}
}

func (p *proxyHandler) effectiveProxyOrigin(r *http.Request) string {
	if p.cfg.ProxyOrigin != "" {
		return p.cfg.ProxyOrigin
	}
	return requestOrigin(r)
}

func (p *proxyHandler) modifyResponse(resp *http.Response) error {
	p.rewriteRedirectLocation(resp)
	return p.modifyLFSResponse(resp)
}

func (p *proxyHandler) rewriteRedirectLocation(resp *http.Response) {
	if resp == nil || resp.Request == nil {
		return
	}
	if resp.StatusCode < http.StatusMultipleChoices || resp.StatusCode >= http.StatusBadRequest {
		return
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return
	}

	origin := p.cfg.ProxyOrigin
	if origin == "" {
		if ctxOrigin, ok := resp.Request.Context().Value(ctxKeyProxyOrigin{}).(string); ok && ctxOrigin != "" {
			origin = ctxOrigin
		}
	}
	if origin == "" {
		return
	}

	locURL, err := url.Parse(location)
	if err != nil {
		return
	}
	if !locURL.IsAbs() {
		return
	}
	if host := locURL.Hostname(); !p.hostAllowed(host) {
		return
	}

	base, err := url.Parse(origin)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return
	}

	locURL.Scheme = base.Scheme
	locURL.Host = base.Host
	locURL.User = nil
	locURL.Path = singleJoinPath(base.Path, locURL.Path)
	locURL.RawPath = ""
	resp.Header.Set("Location", locURL.String())
}

func (p *proxyHandler) resolveUpstreamURL(u *url.URL) (*url.URL, error) {
	clean := strings.TrimPrefix(u.Path, "/")
	if strings.HasPrefix(clean, "http://") || strings.HasPrefix(clean, "https://") {
		target, err := url.Parse(clean)
		if err == nil && target.Scheme != "" && target.Host != "" {
			target.RawQuery = u.RawQuery
			return target, nil
		}
	}

	upstream, err := url.Parse(p.cfg.DefaultUpstream)
	if err != nil {
		return nil, errors.New("DEFAULT_UPSTREAM is invalid")
	}
	upstream.Path = singleJoinPath(upstream.Path, clean)
	upstream.RawQuery = u.RawQuery
	return upstream, nil
}

func (p *proxyHandler) recordUsage(entry logdb.Entry) {
	if p.store == nil {
		return
	}
	if entry.Repo == "" || entry.Bytes <= 0 {
		return
	}
	if p.usageAgg != nil {
		p.usageAgg.Add(entry)
		return
	}
	if err := p.store.RecordUsage(context.Background(), entry); err != nil {
		log.Printf("[usage-log] failed to record: %v", err)
	}
}

func (p *proxyHandler) logUsage(entry logdb.Entry, status int) {
	duration := entry.Duration
	if duration < 0 {
		duration = 0
	}
	if rounded := duration.Round(time.Millisecond); rounded > 0 {
		duration = rounded
	}
	log.Printf("[usage] ip=%s repo=%s bytes=%d status=%d method=%s path=%s upstream=%s duration=%s",
		entry.ClientIP,
		entry.Repo,
		entry.Bytes,
		status,
		entry.Method,
		entry.RequestPath,
		entry.Upstream,
		duration)
}
