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

func TestStore_RecordRequestAndResponse(t *testing.T) {
	s := newUsageStore()
	s.RecordRequest("2026-04-16", "tok", "modelA", "2026-04-16T00:00:00Z")
	s.RecordRequest("2026-04-16", "tok", "modelA", "2026-04-16T00:01:00Z")
	s.RecordResponse("2026-04-16", "tok", "modelA", ollamaUsage{
		PromptEvalCount: 10,
		EvalCount:       20,
		TotalDuration:   100,
	})

	snap := s.Snapshot()
	st := snap["2026-04-16"]["tok"]["modelA"]
	if st.RequestCount != 2 {
		t.Errorf("request_count: want 2, got %d", st.RequestCount)
	}
	if st.ResponseCount != 1 {
		t.Errorf("response_count: want 1, got %d", st.ResponseCount)
	}
	if st.PromptEvalCount != 10 {
		t.Errorf("prompt_eval_count: want 10, got %d", st.PromptEvalCount)
	}
	if st.EvalCount != 20 {
		t.Errorf("eval_count: want 20, got %d", st.EvalCount)
	}
	if st.TotalDuration != 100 {
		t.Errorf("total_duration: want 100, got %d", st.TotalDuration)
	}
	if st.LastRequestTimestamp != "2026-04-16T00:01:00Z" {
		t.Errorf("last_request_timestamp: want %q, got %q", "2026-04-16T00:01:00Z", st.LastRequestTimestamp)
	}
}

func TestStore_MultipleTokensAndDates(t *testing.T) {
	s := newUsageStore()
	s.RecordRequest("2026-04-16", "tok-a", "m1", "2026-04-16T00:00:00Z")
	s.RecordRequest("2026-04-17", "tok-b", "m1", "2026-04-17T00:00:00Z")
	s.RecordResponse("2026-04-16", "tok-a", "m1", ollamaUsage{PromptEvalCount: 5, EvalCount: 15})

	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 dates, got %d", len(snap))
	}
	st := snap["2026-04-16"]["tok-a"]["m1"]
	if st.PromptEvalCount+st.EvalCount != 20 {
		t.Error("wrong total eval count for tok-a on 2026-04-16")
	}
	if snap["2026-04-17"]["tok-b"]["m1"].RequestCount != 1 {
		t.Error("wrong request_count for tok-b on 2026-04-17")
	}
}

func TestStore_MultiModelBucketing(t *testing.T) {
	s := newUsageStore()
	s.RecordRequest("2026-04-17", "tok", "model-a", "2026-04-17T00:00:00Z")
	s.RecordRequest("2026-04-17", "tok", "model-b", "2026-04-17T00:00:01Z")
	s.RecordResponse("2026-04-17", "tok", "model-a", ollamaUsage{EvalCount: 10})
	s.RecordResponse("2026-04-17", "tok", "model-b", ollamaUsage{EvalCount: 20})

	snap := s.Snapshot()
	models := snap["2026-04-17"]["tok"]
	if len(models) != 2 {
		t.Fatalf("expected 2 model buckets, got %d", len(models))
	}
	if models["model-a"].EvalCount != 10 {
		t.Errorf("model-a eval_count: want 10, got %d", models["model-a"].EvalCount)
	}
	if models["model-b"].EvalCount != 20 {
		t.Errorf("model-b eval_count: want 20, got %d", models["model-b"].EvalCount)
	}
}

func TestStore_ResponseCountVsRequestCount(t *testing.T) {
	s := newUsageStore()
	s.RecordRequest("2026-04-17", "tok", "m", "2026-04-17T00:00:00Z")
	s.RecordRequest("2026-04-17", "tok", "m", "2026-04-17T00:00:01Z")
	s.RecordRequest("2026-04-17", "tok", "m", "2026-04-17T00:00:02Z")
	s.RecordResponse("2026-04-17", "tok", "m", ollamaUsage{EvalCount: 5})
	s.RecordResponse("2026-04-17", "tok", "m", ollamaUsage{EvalCount: 5})

	snap := s.Snapshot()
	st := snap["2026-04-17"]["tok"]["m"]
	if st.RequestCount != 3 {
		t.Errorf("request_count: want 3, got %d", st.RequestCount)
	}
	if st.ResponseCount != 2 {
		t.Errorf("response_count: want 2, got %d", st.ResponseCount)
	}
}

func TestStore_LastRequestTimestampUpdated(t *testing.T) {
	s := newUsageStore()
	s.RecordRequest("2026-04-17", "tok", "m", "2026-04-17T00:00:00Z")
	s.RecordRequest("2026-04-17", "tok", "m", "2026-04-17T12:00:00Z")

	snap := s.Snapshot()
	ts := snap["2026-04-17"]["tok"]["m"].LastRequestTimestamp
	if ts != "2026-04-17T12:00:00Z" {
		t.Errorf("last_request_timestamp: want %q, got %q", "2026-04-17T12:00:00Z", ts)
	}
}

func TestStore_TimingFieldsAccumulated(t *testing.T) {
	s := newUsageStore()
	s.RecordResponse("2026-04-17", "tok", "m", ollamaUsage{
		TotalDuration:      1000,
		LoadDuration:       200,
		PromptEvalCount:    10,
		PromptEvalDuration: 50,
		EvalCount:          100,
		EvalDuration:       750,
	})
	s.RecordResponse("2026-04-17", "tok", "m", ollamaUsage{
		TotalDuration:      2000,
		LoadDuration:       400,
		PromptEvalCount:    20,
		PromptEvalDuration: 100,
		EvalCount:          200,
		EvalDuration:       1500,
	})

	snap := s.Snapshot()
	st := snap["2026-04-17"]["tok"]["m"]
	if st.TotalDuration != 3000 {
		t.Errorf("total_duration: want 3000, got %d", st.TotalDuration)
	}
	if st.LoadDuration != 600 {
		t.Errorf("load_duration: want 600, got %d", st.LoadDuration)
	}
	if st.PromptEvalCount != 30 {
		t.Errorf("prompt_eval_count: want 30, got %d", st.PromptEvalCount)
	}
	if st.PromptEvalDuration != 150 {
		t.Errorf("prompt_eval_duration: want 150, got %d", st.PromptEvalDuration)
	}
	if st.EvalCount != 300 {
		t.Errorf("eval_count: want 300, got %d", st.EvalCount)
	}
	if st.EvalDuration != 2250 {
		t.Errorf("eval_duration: want 2250, got %d", st.EvalDuration)
	}
}

// --- inspectingReader tests ---

func TestInspectingReader_NonStreamingUsage(t *testing.T) {
	s := newUsageStore()
	body := `{"model":"llama3","response":"Hi","prompt_eval_count":7,"eval_count":13}`
	r := newInspectingReader(io.NopCloser(strings.NewReader(body)), s, "tok", "llama3")

	io.ReadAll(r)
	r.Close()

	snap := s.Snapshot()
	for _, keys := range snap {
		for _, models := range keys {
			st, ok := models["llama3"]
			if !ok {
				continue
			}
			if st.PromptEvalCount != 7 || st.EvalCount != 13 {
				t.Errorf("want prompt=7 eval=13, got prompt=%d eval=%d",
					st.PromptEvalCount, st.EvalCount)
			}
			return
		}
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
	r := newInspectingReader(io.NopCloser(strings.NewReader(ndjson)), s, "tok", "llama3")
	io.ReadAll(r)
	r.Close()

	snap := s.Snapshot()
	for _, keys := range snap {
		for _, models := range keys {
			st, ok := models["llama3"]
			if !ok {
				continue
			}
			if st.PromptEvalCount != 10 || st.EvalCount != 25 {
				t.Errorf("want prompt=10 eval=25, got prompt=%d eval=%d",
					st.PromptEvalCount, st.EvalCount)
			}
			return
		}
	}
	t.Error("no usage recorded")
}

func TestInspectingReader_DoneFalseNotRecorded(t *testing.T) {
	s := newUsageStore()
	ndjson := strings.Join([]string{
		`{"done":false,"prompt_eval_count":5,"eval_count":10}`,
		`{"done":false,"prompt_eval_count":5,"eval_count":10}`,
		"",
	}, "\n")
	r := newInspectingReader(io.NopCloser(strings.NewReader(ndjson)), s, "tok", "m")
	io.ReadAll(r)
	r.Close()

	snap := s.Snapshot()
	for _, keys := range snap {
		for _, models := range keys {
			if st, ok := models["m"]; ok && st.EvalCount > 0 {
				t.Errorf("expected no usage recorded, got %+v", st)
			}
		}
	}
}

func TestInspectingReader_PassesBytesThrough(t *testing.T) {
	s := newUsageStore()
	original := `{"done":true,"prompt_eval_count":1,"eval_count":2}` + "\n"
	r := newInspectingReader(io.NopCloser(strings.NewReader(original)), s, "tok", "m")
	got, _ := io.ReadAll(r)
	r.Close()
	if string(got) != original {
		t.Errorf("body modified: want %q, got %q", original, string(got))
	}
}

func TestInspectingReader_ModelRouting(t *testing.T) {
	s := newUsageStore()

	bodyA := `{"done":true,"prompt_eval_count":5,"eval_count":10}` + "\n"
	rA := newInspectingReader(io.NopCloser(strings.NewReader(bodyA)), s, "tok", "model-a")
	io.ReadAll(rA)
	rA.Close()

	bodyB := `{"done":true,"prompt_eval_count":3,"eval_count":7}` + "\n"
	rB := newInspectingReader(io.NopCloser(strings.NewReader(bodyB)), s, "tok", "model-b")
	io.ReadAll(rB)
	rB.Close()

	snap := s.Snapshot()
	var found int
	for _, keys := range snap {
		for _, models := range keys {
			if st, ok := models["model-a"]; ok {
				found++
				if st.ResponseCount != 1 || st.EvalCount != 10 {
					t.Errorf("model-a: want response_count=1 eval=10, got %+v", st)
				}
			}
			if st, ok := models["model-b"]; ok {
				found++
				if st.ResponseCount != 1 || st.EvalCount != 7 {
					t.Errorf("model-b: want response_count=1 eval=7, got %+v", st)
				}
			}
		}
	}
	if found != 2 {
		t.Errorf("expected 2 model buckets recorded, found %d", found)
	}
}

// --- Proxy + usage endpoint integration ---

func TestProxyIntegration_NonStreamingUsageCaptured(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization header should be stripped before forwarding")
		}
		fmt.Fprintln(w, `{"done":true,"prompt_eval_count":8,"eval_count":16}`)
	}))
	defer backend.Close()

	store, proxy, mux := buildTestServer(t, backend.URL)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/generate", strings.NewReader(`{"model":"test-model"}`))
	req.Header.Set("Authorization", "Bearer good-token")
	mux.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusOK)

	io.ReadAll(rec.Body)

	snap := store.Snapshot()
	for _, keys := range snap {
		if models, ok := keys["good-token"]; ok {
			for _, st := range models {
				if st.RequestCount >= 1 {
					_ = proxy
					return
				}
			}
		}
	}
	t.Error("no usage recorded after proxied request")
}

func TestProxyIntegration_ModelExtracted(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"done":true,"prompt_eval_count":1,"eval_count":2}`)
	}))
	defer backend.Close()

	store, _, mux := buildTestServer(t, backend.URL)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/generate", strings.NewReader(`{"model":"test-model"}`))
	req.Header.Set("Authorization", "Bearer good-token")
	mux.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusOK)
	io.ReadAll(rec.Body)

	snap := store.Snapshot()
	for _, keys := range snap {
		if models, ok := keys["good-token"]; ok {
			if _, ok := models["test-model"]; ok {
				return
			}
		}
	}
	t.Error("expected 'test-model' bucket in snapshot")
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

	store.RecordRequest("2026-04-16", "good-token", "llama3", "2026-04-16T00:00:00Z")
	store.RecordResponse("2026-04-16", "good-token", "llama3", ollamaUsage{PromptEvalCount: 3, EvalCount: 7})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/usage", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	mux.ServeHTTP(rec, req)
	assertStatus(t, rec, http.StatusOK)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}

	var body map[string]map[string]map[string]map[string]UsageStat
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
	tokenEntry, ok := dateEntry["good-token"]
	if !ok {
		t.Fatal("response missing token key 'good-token'")
	}
	st, ok := tokenEntry["llama3"]
	if !ok {
		t.Fatal("response missing model key 'llama3'")
	}
	if st.RequestCount != 1 || st.PromptEvalCount != 3 || st.EvalCount != 7 || st.ResponseCount != 1 {
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
	for _, keys := range snap {
		for _, models := range keys {
			for _, st := range models {
				totalRequests += st.RequestCount
			}
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
