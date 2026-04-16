package main

import (
	"encoding/json"
	"net/http"
)

// usageHandler returns a handler that serves a JSON snapshot of accumulated usage.
func usageHandler(store *UsageStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot := store.Snapshot()
		resp := map[string]any{"usage": snapshot}
		data, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data) //nolint:errcheck
	}
}
