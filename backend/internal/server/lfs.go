package server

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (p *proxyHandler) modifyLFSResponse(resp *http.Response) error {
	if !strings.Contains(resp.Request.URL.Path, "/info/lfs/objects/batch") {
		return nil
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "json") {
		return nil
	}

	repo := extractRepoFromPath(resp.Request.URL.Path)

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}

	ungzipped, wasGzipped := maybeGunzip(resp, body)
	base := p.cfg.ProxyOrigin
	if base == "" && resp.Request != nil {
		base, _ = resp.Request.Context().Value(ctxKeyProxyOrigin{}).(string)
	}
	rewritten, err := p.rewriteLFSBatchJSON(ungzipped, repo, base)
	if err != nil {
		return err
	}

	packed, encoding := maybeGzip(rewritten, wasGzipped)
	resp.Body = io.NopCloser(bytes.NewReader(packed))
	resp.ContentLength = int64(len(packed))
	resp.Header.Del("Content-Length")
	if encoding != "" {
		resp.Header.Set("Content-Encoding", encoding)
	} else {
		resp.Header.Del("Content-Encoding")
	}
	return nil
}

type lfsAction struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresAt string            `json:"expires_at,omitempty"`
}

type lfsObj struct {
	Oid     string               `json:"oid"`
	Size    int64                `json:"size"`
	Actions map[string]lfsAction `json:"actions,omitempty"`
}

type lfsBatch struct {
	Transfer string   `json:"transfer,omitempty"`
	Objects  []lfsObj `json:"objects"`
}

func (p *proxyHandler) rewriteLFSBatchJSON(body []byte, repo string, origin string) ([]byte, error) {
	if origin == "" {
		return body, nil
	}

	var payload lfsBatch
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil
	}

	base, err := url.Parse(origin)
	if err != nil {
		return body, nil
	}

	for i := range payload.Objects {
		obj := &payload.Objects[i]
		for name, action := range obj.Actions {
			u, err := url.Parse(action.Href)
			if err != nil || u.Scheme == "" || u.Host == "" {
				continue
			}
			if !p.hostAllowed(u.Host) {
				continue
			}

			next := *base
			next.Path = singleJoinPath(next.Path, "/lfs/content")
			q := next.Query()
			q.Set("u", action.Href)
			if repo != "" {
				q.Set("repo", repo)
			}
			next.RawQuery = q.Encode()
			action.Href = next.String()
			obj.Actions[name] = action
		}
	}

	out, err := json.Marshal(payload)
	if err != nil {
		return body, nil
	}
	return out, nil
}

func maybeGunzip(resp *http.Response, body []byte) ([]byte, bool) {
	if strings.ToLower(resp.Header.Get("Content-Encoding")) != "gzip" {
		return body, false
	}

	gr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return body, false
	}
	defer gr.Close()

	data, err := io.ReadAll(gr)
	if err != nil {
		return body, false
	}
	return data, true
}

func maybeGzip(data []byte, want bool) ([]byte, string) {
	if !want {
		return data, ""
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(data)
	_ = zw.Close()
	return buf.Bytes(), "gzip"
}
