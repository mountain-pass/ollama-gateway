package main

import "sync"

// UsageStat holds accumulated counters for one (date, token) pair.
type UsageStat struct {
	Requests         int64 `json:"requests"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// UsageStore is a thread-safe in-memory store keyed by ISO date then API token.
type UsageStore struct {
	mu   sync.Mutex
	data map[string]map[string]*UsageStat // [isodate][token]
}

func newUsageStore() *UsageStore {
	return &UsageStore{data: make(map[string]map[string]*UsageStat)}
}

// RecordRequest increments the request counter for the given date and token.
func (s *UsageStore) RecordRequest(date, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreate(date, token).Requests++
}

// RecordUsage adds prompt and completion token counts for the given date and token.
func (s *UsageStore) RecordUsage(date, token string, prompt, completion int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.getOrCreate(date, token)
	st.PromptTokens += prompt
	st.CompletionTokens += completion
	st.TotalTokens += prompt + completion
}

// Snapshot returns a deep copy of the store contents safe for JSON marshalling.
func (s *UsageStore) Snapshot() map[string]map[string]UsageStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]map[string]UsageStat, len(s.data))
	for date, tokens := range s.data {
		out[date] = make(map[string]UsageStat, len(tokens))
		for token, st := range tokens {
			out[date][token] = *st
		}
	}
	return out
}

// getOrCreate returns the UsageStat pointer for (date, token), creating it if needed.
// Caller must hold s.mu.
func (s *UsageStore) getOrCreate(date, token string) *UsageStat {
	if s.data[date] == nil {
		s.data[date] = make(map[string]*UsageStat)
	}
	if s.data[date][token] == nil {
		s.data[date][token] = &UsageStat{}
	}
	return s.data[date][token]
}
