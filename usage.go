package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// usageHandler returns a handler that serves a JSON snapshot of accumulated usage,
// optionally filtered by path segments: /usage, /usage/{date}, /usage/{date}/{user},
// /usage/{date}/{user}/{model}.
func usageHandler(store *UsageStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/usage")
		rest = strings.Trim(rest, "/")
		var segments []string
		if rest != "" {
			segments = strings.Split(rest, "/")
		}
		for i, s := range segments {
			if dec, err := url.PathUnescape(s); err == nil {
				segments[i] = dec
			}
		}

		snapshot := store.Snapshot()
		var result any
		switch len(segments) {
		case 0:
			result = snapshot
		case 1:
			v, ok := snapshot[segments[0]]
			if !ok {
				jsonNotFound(w)
				return
			}
			result = v
		case 2:
			d, ok := snapshot[segments[0]]
			if !ok {
				jsonNotFound(w)
				return
			}
			v, ok := d[segments[1]]
			if !ok {
				jsonNotFound(w)
				return
			}
			result = v
		case 3:
			d, ok := snapshot[segments[0]]
			if !ok {
				jsonNotFound(w)
				return
			}
			u, ok := d[segments[1]]
			if !ok {
				jsonNotFound(w)
				return
			}
			v, ok := u[segments[2]]
			if !ok {
				jsonNotFound(w)
				return
			}
			result = v
		default:
			http.NotFound(w, r)
			return
		}

		data, err := json.Marshal(result)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data) //nolint:errcheck
	}
}

func jsonNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{"error":"not found"}`)) //nolint:errcheck
}
