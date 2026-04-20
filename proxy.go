package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

type proxyContextKey int

const ctxKeyModel proxyContextKey = iota

// extractModel reads the request body, extracts the "model" JSON field, and
// restores the body so the proxy can still forward it.
func extractModel(req *http.Request) string {
	if req.Body == nil {
		return "unknown"
	}
	data, err := io.ReadAll(req.Body)
	req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(data))
	if err != nil || len(data) == 0 {
		return "unknown"
	}
	var v struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(data, &v) != nil || v.Model == "" {
		return "unknown"
	}
	return v.Model
}

// newReverseProxy builds a reverse proxy that forwards requests to target,
// strips the Authorization header, records request counts, and wraps response
// bodies with an inspectingReader to capture Ollama token usage.
func newReverseProxy(target *url.URL, store *UsageStore) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Flush each chunk immediately so streaming responses reach the client
	// without buffering.
	proxy.FlushInterval = -1

	baseDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		model := extractModel(req)
		*req = *req.WithContext(context.WithValue(req.Context(), ctxKeyModel, model))
		baseDirector(req)
		// Strip auth header — Ollama does not require it.
		req.Header.Del("Authorization")

		// Count the request here so it is recorded even if Ollama never responds.
		token, _ := req.Context().Value(ctxKeyToken).(string)
		now := time.Now().UTC()
		store.RecordRequest(now.Format("2006-01-02"), token, model, now.Format(time.RFC3339))
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		if strings.HasPrefix(resp.Request.URL.Path, "/usage") {
			return nil
		}
		token, _ := resp.Request.Context().Value(ctxKeyToken).(string)
		model, _ := resp.Request.Context().Value(ctxKeyModel).(string)
		// Wrap the body so usage fields are captured as the response streams.
		resp.Body = newInspectingReader(resp.Body, store, token, model)
		return nil
	}

	return proxy
}
