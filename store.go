package main

import "sync"

// UsageStat holds accumulated counters for one (date, apikey, model) bucket.
type UsageStat struct {
	RequestCount         int64  `json:"request_count"`
	ResponseCount        int64  `json:"response_count"`
	LastRequestTimestamp string `json:"last_request_timestamp"`
	TotalDuration        int64  `json:"total_duration"`
	LoadDuration         int64  `json:"load_duration"`
	PromptEvalCount      int64  `json:"prompt_eval_count"`
	PromptEvalDuration   int64  `json:"prompt_eval_duration"`
	EvalCount            int64  `json:"eval_count"`
	EvalDuration         int64  `json:"eval_duration"`
	TotalTokens          int64  `json:"total_tokens"`
}

// UsageStore is a thread-safe in-memory store keyed by ISO date, API key, then model.
type UsageStore struct {
	mu   sync.Mutex
	data map[string]map[string]map[string]*UsageStat // [isodate][apikey][model]
}

func newUsageStore() *UsageStore {
	return &UsageStore{data: make(map[string]map[string]map[string]*UsageStat)}
}

// RecordRequest increments request_count and updates last_request_timestamp.
func (s *UsageStore) RecordRequest(date, token, model, timestamp string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.getOrCreate(date, token, model)
	st.RequestCount++
	st.LastRequestTimestamp = timestamp
}

// RecordResponse increments response_count and accumulates all timing/token fields.
func (s *UsageStore) RecordResponse(date, token, model string, u ollamaUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.getOrCreate(date, token, model)
	st.ResponseCount++
	st.TotalDuration += u.TotalDuration
	st.LoadDuration += u.LoadDuration
	st.PromptEvalCount += u.PromptEvalCount
	st.PromptEvalDuration += u.PromptEvalDuration
	st.EvalCount += u.EvalCount
	st.EvalDuration += u.EvalDuration
	st.TotalTokens += u.PromptEvalCount + u.EvalCount
}

// Snapshot returns a deep copy of the store contents safe for JSON marshalling.
func (s *UsageStore) Snapshot() map[string]map[string]map[string]UsageStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]map[string]map[string]UsageStat, len(s.data))
	for date, keys := range s.data {
		out[date] = make(map[string]map[string]UsageStat, len(keys))
		for token, models := range keys {
			out[date][token] = make(map[string]UsageStat, len(models))
			for model, st := range models {
				out[date][token][model] = *st
			}
		}
	}
	return out
}

// getOrCreate returns the UsageStat pointer for (date, token, model), creating entries as needed.
// Caller must hold s.mu.
func (s *UsageStore) getOrCreate(date, token, model string) *UsageStat {
	if s.data[date] == nil {
		s.data[date] = make(map[string]map[string]*UsageStat)
	}
	if s.data[date][token] == nil {
		s.data[date][token] = make(map[string]*UsageStat)
	}
	if s.data[date][token][model] == nil {
		s.data[date][token][model] = &UsageStat{}
	}
	return s.data[date][token][model]
}
