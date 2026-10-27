package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIStreamCountsTokens(t *testing.T) {
	var think any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		think = body["think"]
		// usage chunk arrives AFTER finish_reason, as in the OpenAI API
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Halo\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" dunia\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":42}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "")
	ch, err := p.Stream(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	var last StreamChunk
	for c := range ch {
		text.WriteString(c.Delta)
		if c.FinishReason != "" {
			last = c
		}
	}
	if text.String() != "Halo dunia" {
		t.Fatalf("text = %q", text.String())
	}
	if last.OutputTokens != 42 || last.InputTokens != 5 {
		t.Fatalf("tokens = in %d out %d, want 5/42", last.InputTokens, last.OutputTokens)
	}
	if think != false {
		t.Fatalf("think = %v, want false", think)
	}
}

func TestOpenAIStreamFallbackCountsDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 7; i++ {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	ch, err := NewOpenAIProvider(srv.URL, "").Stream(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	var last StreamChunk
	for c := range ch {
		if c.FinishReason != "" {
			last = c
		}
	}
	if last.OutputTokens != 7 {
		t.Fatalf("out = %d, want 7", last.OutputTokens)
	}
}

func TestOpenAIStreamTimingIncludesReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning\":\"hmm\"}}]}\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(300 * time.Millisecond)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":3}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	ch, err := NewOpenAIProvider(srv.URL, "").Stream(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	var last StreamChunk
	for c := range ch {
		if c.FinishReason != "" {
			last = c
		}
	}
	// 3 tokens over >=0.3s must be <= 10 t/s (would be far higher if the timer started at the first content token)
	if last.TokensPerSec <= 0 || last.TokensPerSec > 10.5 {
		t.Fatalf("tokens/sec = %v, want ~10 or less", last.TokensPerSec)
	}
}
