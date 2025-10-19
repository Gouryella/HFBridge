package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func (p *proxyHandler) withUpstreamAuth(req *http.Request) {
	if p.cfg.HFToken == "" {
		return
	}
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.HFToken)
	}
}

func (p *proxyHandler) hostAllowed(host string) bool {
	host = strings.ToLower(host)
	for _, pattern := range p.cfg.AllowHosts {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if host == pattern {
			return true
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*.")
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				return true
			}
		}
	}
	return false
}

func copyHeadersPassThrough(dst, src http.Header) {
	for k, vv := range src {
		switch strings.ToLower(k) {
		case "host", "connection", "keep-alive", "proxy-authenticate",
			"proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
			continue
		default:
			for _, v := range vv {
				dst.Add(k, v)
			}
		}
	}
}

func singleJoinPath(a, b string) string {
	if strings.HasSuffix(a, "/") {
		return a + strings.TrimPrefix(b, "/")
	}
	return a + "/" + strings.TrimPrefix(b, "/")
}

func requestPathWithQuery(path, rawQuery string) string {
	if rawQuery == "" {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + rawQuery
}

func extractRepoFromPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}

	segments := strings.Split(p, "/")
	filtered := segments[:0]
	for _, segment := range segments {
		if segment != "" {
			filtered = append(filtered, segment)
		}
	}
	if len(filtered) < 2 {
		return ""
	}

	i := 0
	if filtered[i] == "api" {
		i++
		if i >= len(filtered) {
			return ""
		}
	}
	if filtered[i] == "models" || filtered[i] == "datasets" || filtered[i] == "spaces" || filtered[i] == "repos" {
		if len(filtered)-(i+1) >= 2 {
			i++
		}
	}
	if i+1 >= len(filtered) {
		return ""
	}

	owner := filtered[i]
	repo := strings.TrimSuffix(filtered[i+1], ".git")
	if owner == "" || repo == "" {
		return ""
	}
	return owner + "/" + repo
}

func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		if idx := strings.Index(xf, ","); idx != -1 {
			return normalizeIP(strings.TrimSpace(xf[:idx]))
		}
		return normalizeIP(strings.TrimSpace(xf))
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return normalizeIP(strings.TrimSpace(xr))
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return normalizeIP(host)
	}
	return normalizeIP(r.RemoteAddr)
}

func normalizeIP(s string) string {
	if s == "" {
		return s
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return s
	}
	if ip.IsLoopback() {
		return "127.0.0.1"
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return strings.ToLower(ip.String())
}

func requestOrigin(r *http.Request) string {
	if r == nil {
		return ""
	}
	proto := forwardedProto(r)
	host := forwardedHost(r)
	if host == "" {
		return ""
	}
	return proto + "://" + host
}

func forwardedProto(r *http.Request) string {
	if r == nil {
		return "http"
	}
	if proto := headerFirst(r.Header, "Forwarded"); proto != "" {
		if v := extractForwardedParam(proto, "proto"); v != "" {
			return strings.ToLower(v)
		}
	}
	for _, key := range []string{"X-Forwarded-Proto", "X-Forwarded-Scheme"} {
		if v := headerFirst(r.Header, key); v != "" {
			return strings.ToLower(v)
		}
	}
	if r.TLS != nil {
		return "https"
	}
	if r.URL != nil && r.URL.Scheme != "" {
		return strings.ToLower(r.URL.Scheme)
	}
	return "http"
}

func forwardedHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	if forwarded := headerFirst(r.Header, "Forwarded"); forwarded != "" {
		if v := extractForwardedParam(forwarded, "host"); v != "" {
			return v
		}
	}
	for _, key := range []string{"X-Forwarded-Host", "Host"} {
		if v := headerFirst(r.Header, key); v != "" {
			return v
		}
	}
	if r.Host != "" {
		return r.Host
	}
	if v := headerFirst(r.Header, "X-Forwarded-Server"); v != "" {
		return v
	}
	return ""
}

func headerFirst(h http.Header, key string) string {
	if h == nil {
		return ""
	}
	values := h.Values(key)
	if len(values) == 0 {
		return ""
	}
	value := values[0]
	if idx := strings.Index(value, ","); idx != -1 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func extractForwardedParam(forwarded, name string) string {
	for forwarded != "" {
		part := forwarded
		if idx := strings.Index(part, ","); idx != -1 {
			part = part[:idx]
			forwarded = strings.TrimSpace(forwarded[idx+1:])
		} else {
			forwarded = ""
		}
		part = strings.TrimSpace(part)
		params := strings.Split(part, ";")
		for _, p := range params {
			if eq := strings.Index(p, "="); eq != -1 {
				key := strings.TrimSpace(p[:eq])
				if strings.EqualFold(key, name) {
					val := strings.TrimSpace(p[eq+1:])
					val = strings.Trim(val, `"`)
					return val
				}
			}
		}
	}
	return ""
}

type countingResponseWriter struct {
	http.ResponseWriter
	stats   *statsCollector
	written int64
	status  int
}

func newCountingResponseWriter(w http.ResponseWriter, stats *statsCollector) *countingResponseWriter {
	return &countingResponseWriter{
		ResponseWriter: w,
		stats:          stats,
	}
}

func (w *countingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	if n > 0 {
		w.written += int64(n)
		if w.stats != nil {
			w.stats.Record(int64(n))
		}
	}
	return n, err
}

func (w *countingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *countingResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	cr := &countingReader{r: r}
	var (
		n   int64
		err error
	)
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err = rf.ReadFrom(cr)
	} else {
		n, err = io.CopyBuffer(w.ResponseWriter, cr, nil)
	}
	if cr.n > 0 {
		w.written += cr.n
		if w.stats != nil {
			w.stats.Record(cr.n)
		}
	}
	return n, err
}

func (w *countingResponseWriter) BytesWritten() int64 {
	return w.written
}

func (w *countingResponseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *countingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *countingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacker not supported")
}

func (w *countingResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

type countingReader struct {
	r io.Reader
	n int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	if n > 0 {
		cr.n += int64(n)
	}
	return n, err
}
