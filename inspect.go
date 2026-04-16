package main

import (
	"bytes"
	"encoding/json"
	"io"
	"time"
)

// ollamaUsage is the minimal shape we look for in every JSON object from Ollama.
type ollamaUsage struct {
	Done            *bool `json:"done"` // pointer so we can detect absence
	PromptEvalCount int64 `json:"prompt_eval_count"`
	EvalCount       int64 `json:"eval_count"`
}

// inspectingReader wraps an Ollama response body.  It passes bytes through to
// the caller unmodified while scanning each newline-terminated JSON object for
// usage fields.  It works for both streaming (NDJSON) and single-object bodies.
type inspectingReader struct {
	body  io.ReadCloser
	buf   []byte
	store *UsageStore
	date  string
	token string
}

func newInspectingReader(body io.ReadCloser, store *UsageStore, token string) *inspectingReader {
	return &inspectingReader{
		body:  body,
		store: store,
		date:  time.Now().UTC().Format("2006-01-02"),
		token: token,
	}
}

func (r *inspectingReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 {
		r.buf = append(r.buf, p[:n]...)
		r.scanLines()
	}
	return n, err
}

func (r *inspectingReader) Close() error {
	// Flush any remaining bytes — handles non-streaming single-object responses
	// that are not terminated by a newline.
	if len(r.buf) > 0 {
		r.tryRecord(r.buf)
		r.buf = nil
	}
	return r.body.Close()
}

// scanLines extracts and processes all complete newline-terminated JSON objects
// from the buffer, leaving any incomplete trailing fragment in place.
func (r *inspectingReader) scanLines() {
	for {
		idx := bytes.IndexByte(r.buf, '\n')
		if idx < 0 {
			break
		}
		line := r.buf[:idx]
		r.buf = r.buf[idx+1:]
		if len(bytes.TrimSpace(line)) > 0 {
			r.tryRecord(line)
		}
	}
}

// tryRecord attempts to parse a JSON fragment and record usage if appropriate.
func (r *inspectingReader) tryRecord(data []byte) {
	var u ollamaUsage
	if err := json.Unmarshal(data, &u); err != nil {
		return // not JSON or malformed — ignore
	}

	// Record usage when:
	//   a) done == true  (final object in a streaming response), or
	//   b) done field is absent and there are non-zero token counts
	//      (non-streaming single-object response).
	shouldRecord := false
	if u.Done != nil {
		shouldRecord = *u.Done
	} else if u.PromptEvalCount > 0 || u.EvalCount > 0 {
		shouldRecord = true
	}

	if shouldRecord {
		r.store.RecordUsage(r.date, r.token, u.PromptEvalCount, u.EvalCount)
	}
}
