package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

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
		baseDirector(req)
		// Strip auth header — Ollama does not require it.
		req.Header.Del("Authorization")
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		token, _ := resp.Request.Context().Value(ctxKeyToken).(string)
		date := time.Now().UTC().Format("2006-01-02")

		// Always count the request, regardless of whether the response has
		// usage fields.
		store.RecordRequest(date, token)

		// Wrap the body so usage fields are captured as the response streams.
		resp.Body = newInspectingReader(resp.Body, store, token)
		return nil
	}

	return proxy
}
