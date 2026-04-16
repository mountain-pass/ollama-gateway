package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// validTokens used across all tests.
var testTokens = map[string]struct{}{
	"good-token": {},
}

// --- Auth middleware tests ---

func TestAuthMiddleware_NoHeader(t *testing.T) {
	h := authMiddleware(testTokens, okHandler())
	rec, req := newReq("GET", "/", "")
	h.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusUnauthorized)
}

func TestAuthMiddleware_WrongScheme(t *testing.T) {
	h := authMiddleware(testTokens, okHandler())
	rec, req := newReq("GET", "/", "Token good-token")
	h.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusUnauthorized)
}

func TestAuthMiddleware_UnknownToken(t *testing.T) {
	h := authMiddleware(testTokens, okHandler())
	rec, req := newReq("GET", "/", "Bearer bad-token")
	h.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusUnauthorized)
}

func TestAuthMiddleware_EmptyToken(t *testing.T) {
	h := authMiddleware(testTokens, okHandler())
	rec, req := newReq("GET", "/", "Bearer ")
	h.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusUnauthorized)
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	h := authMiddleware(testTokens, okHandler())
	rec, req := newReq("GET", "/", "Bearer good-token")
	h.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusOK)
}

func TestAuthMiddleware_TokenInContext(t *testing.T) {
	var gotToken string
	capture := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken, _ = r.Context().Value(ctxKeyToken).(string)
		w.WriteHeader(http.StatusOK)
	})
	h := authMiddleware(testTokens, capture)
	rec, req := newReq("GET", "/", "Bearer good-token")
	h.ServeHTTP(rec, req)
	if gotToken != "good-token" {
		t.Fatalf("expected token in context %q, got %q", "good-token", gotToken)
	}
}

// --- Usage store tests ---

func TestStore_RecordRequestAndUsage(t *testing.T) {
	s := newUsageStore()
	s.RecordRequest("2026-04-16", "tok")
	s.RecordRequest("2026-04-16", "tok")
	s.RecordUsage("2026-04-16", "tok", 10, 20)

	snap := s.Snapshot()
	st := snap["2026-04-16"]["tok"]
	if st.Requests != 2 {
		t.Errorf("requests: want 2, got %d", st.Requests)
	}
	if st.PromptTokens != 10 {
		t.Errorf("prompt_tokens: want 10, got %d", st.PromptTokens)
	}
	if st.CompletionTokens != 20 {
		t.Errorf("completion_tokens: want 20, got %d", st.CompletionTokens)
	}
	if st.TotalTokens != 30 {
		t.Errorf("total_tokens: want 30, got %d", st.TotalTokens)
	}
}

func TestStore_MultipleTokensAndDates(t *testing.T) {
	s := newUsageStore()
	s.RecordRequest("2026-04-16", "tok-a")
	s.RecordRequest("2026-04-17", "tok-b")
	s.RecordUsage("2026-04-16", "tok-a", 5, 15)

	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 dates, got %d", len(snap))
	}
	if snap["2026-04-16"]["tok-a"].TotalTokens != 20 {
		t.Error("wrong total for tok-a on 2026-04-16")
	}
	if snap["2026-04-17"]["tok-b"].Requests != 1 {
		t.Error("wrong requests for tok-b on 2026-04-17")
	}
}

// --- inspectingReader tests ---

func TestInspectingReader_NonStreamingUsage(t *testing.T) {
	s := newUsageStore()
	body := `{"model":"llama3","response":"Hi","prompt_eval_count":7,"eval_count":13}`
	r := newInspectingReader(io.NopCloser(strings.NewReader(body)), s, "tok")

	// Read entire body (simulates proxy reading and forwarding).
	io.ReadAll(r)
	r.Close()

	snap := s.Snapshot()
	for _, tokens := range snap {
		st, ok := tokens["tok"]
		if !ok {
			continue
		}
		if st.PromptTokens != 7 || st.CompletionTokens != 13 {
			t.Errorf("want prompt=7 completion=13, got prompt=%d completion=%d",
				st.PromptTokens, st.CompletionTokens)
		}
		return
	}
	t.Error("no usage recorded")
}

func TestInspectingReader_StreamingUsageCapturedFromDoneTrue(t *testing.T) {
	s := newUsageStore()
	ndjson := strings.Join([]string{
		`{"done":false,"response":"Hello"}`,
		`{"done":false,"response":" world"}`,
		`{"done":true,"response":"","prompt_eval_count":10,"eval_count":25}`,
		"",
	}, "\n")
	r := newInspectingReader(io.NopCloser(strings.NewReader(ndjson)), s, "tok")
	io.ReadAll(r)
	r.Close()

	snap := s.Snapshot()
	for _, tokens := range snap {
		st, ok := tokens["tok"]
		if !ok {
			continue
		}
		if st.PromptTokens != 10 || st.CompletionTokens != 25 {
			t.Errorf("want prompt=10 completion=25, got prompt=%d completion=%d",
				st.PromptTokens, st.CompletionTokens)
		}
		return
	}
	t.Error("no usage recorded")
}

func TestInspectingReader_DoneFalseNotRecorded(t *testing.T) {
	s := newUsageStore()
	// All chunks have done:false — nothing should be recorded.
	ndjson := strings.Join([]string{
		`{"done":false,"prompt_eval_count":5,"eval_count":10}`,
		`{"done":false,"prompt_eval_count":5,"eval_count":10}`,
		"",
	}, "\n")
	r := newInspectingReader(io.NopCloser(strings.NewReader(ndjson)), s, "tok")
	io.ReadAll(r)
	r.Close()

	snap := s.Snapshot()
	for _, tokens := range snap {
		if st, ok := tokens["tok"]; ok && st.TotalTokens > 0 {
			t.Errorf("expected no token usage recorded, got %+v", st)
		}
	}
}

func TestInspectingReader_PassesBytesThrough(t *testing.T) {
	s := newUsageStore()
	original := `{"done":true,"prompt_eval_count":1,"eval_count":2}` + "\n"
	r := newInspectingReader(io.NopCloser(strings.NewReader(original)), s, "tok")
	got, _ := io.ReadAll(r)
	r.Close()
	if string(got) != original {
		t.Errorf("body modified: want %q, got %q", original, string(got))
	}
}

// --- Proxy + usage endpoint integration ---

func TestProxyIntegration_NonStreamingUsageCaptured(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Authorization header was stripped.
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization header should be stripped before forwarding")
		}
		fmt.Fprintln(w, `{"done":true,"prompt_eval_count":8,"eval_count":16}`)
	}))
	defer backend.Close()

	store, proxy, mux := buildTestServer(t, backend.URL)

	// Make a proxied request.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/generate", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer good-token")
	mux.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusOK)

	// Drain body to ensure inspectingReader.Close is called.
	io.ReadAll(rec.Body)

	// Check usage.
	snap := store.Snapshot()
	for _, tokens := range snap {
		st, ok := tokens["good-token"]
		if !ok {
			continue
		}
		if st.Requests < 1 {
			t.Errorf("expected at least 1 request, got %d", st.Requests)
		}
		_ = proxy
		return
	}
	t.Error("no usage recorded after proxied request")
}

func TestUsageEndpoint_Unauthorized(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()

	_, _, mux := buildTestServer(t, backend.URL)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/usage", nil)
	mux.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusUnauthorized)
}

func TestUsageEndpoint_ReturnsJSON(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"done":true,"prompt_eval_count":3,"eval_count":7}`)
	}))
	defer backend.Close()

	store, _, mux := buildTestServer(t, backend.URL)

	// Seed some usage directly so we don't depend on body parsing timing.
	store.RecordRequest("2026-04-16", "good-token")
	store.RecordUsage("2026-04-16", "good-token", 3, 7)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/usage", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	mux.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusOK)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}

	var body map[string]map[string]map[string]UsageStat
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode /usage response: %v", err)
	}
	usage, ok := body["usage"]
	if !ok {
		t.Fatal("response missing 'usage' key")
	}
	dateEntry, ok := usage["2026-04-16"]
	if !ok {
		t.Fatal("response missing date key '2026-04-16'")
	}
	st, ok := dateEntry["good-token"]
	if !ok {
		t.Fatal("response missing token key 'good-token'")
	}
	if st.Requests != 1 || st.PromptTokens != 3 || st.CompletionTokens != 7 || st.TotalTokens != 10 {
		t.Errorf("unexpected stat: %+v", st)
	}
}

func TestRequestCounter_IncrementedPerRequest(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{}`)
	}))
	defer backend.Close()

	store, _, mux := buildTestServer(t, backend.URL)

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/tags", nil)
		req.Header.Set("Authorization", "Bearer good-token")
		mux.ServeHTTP(rec, req)
	}

	snap := store.Snapshot()
	var totalRequests int64
	for _, tokens := range snap {
		if st, ok := tokens["good-token"]; ok {
			totalRequests += st.Requests
		}
	}
	if totalRequests != 3 {
		t.Errorf("expected 3 requests, got %d", totalRequests)
	}
}

// --- Helpers ---

func buildTestServer(t *testing.T, backendURL string) (*UsageStore, http.Handler, *http.ServeMux) {
	t.Helper()
	target, err := parseURL(backendURL)
	if err != nil {
		t.Fatalf("invalid backend URL: %v", err)
	}
	store := newUsageStore()
	proxy := newReverseProxy(target, store)
	mux := http.NewServeMux()
	mux.Handle("/usage", authMiddleware(testTokens, usageHandler(store)))
	mux.Handle("/", authMiddleware(testTokens, proxy))
	return store, proxy, mux
}

func newReq(method, path, auth string) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(method, path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	return httptest.NewRecorder(), req
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("status: want %d, got %d (body: %s)", want, rec.Code, rec.Body.String())
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
