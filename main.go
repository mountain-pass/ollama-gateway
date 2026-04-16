package main

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func main() {
	ollamaBaseURL := os.Getenv("OLLAMA_BASE_URL")
	if ollamaBaseURL == "" {
		log.Fatal("OLLAMA_BASE_URL environment variable is required")
	}

	apiTokensRaw := os.Getenv("API_TOKENS")
	if apiTokensRaw == "" {
		log.Fatal("API_TOKENS environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	target, err := url.Parse(ollamaBaseURL)
	if err != nil {
		log.Fatalf("Invalid OLLAMA_BASE_URL %q: %v", ollamaBaseURL, err)
	}

	validTokens := parseTokens(apiTokensRaw)
	store := newUsageStore()
	proxy := newReverseProxy(target, store)

	mux := http.NewServeMux()

	// /usage is reserved — served by this proxy, not forwarded to Ollama.
	mux.Handle("/usage", authMiddleware(validTokens, usageHandler(store)))

	// All other requests are proxied through.
	mux.Handle("/", authMiddleware(validTokens, proxy))

	addr := ":" + port
	log.Printf("ollama-gateway listening on %s, proxying to %s", addr, ollamaBaseURL)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// parseURL wraps url.Parse with a cleaner name for use in tests.
func parseURL(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

// parseTokens splits a comma-separated token string into a set for O(1) lookup.
func parseTokens(raw string) map[string]struct{} {
	parts := strings.Split(raw, ",")
	tokens := make(map[string]struct{}, len(parts))
	for _, t := range parts {
		t = strings.TrimSpace(t)
		if t != "" {
			tokens[t] = struct{}{}
		}
	}
	return tokens
}
