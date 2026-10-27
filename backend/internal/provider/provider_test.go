package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIStreamCountsTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// usage chunk arrives AFTER finish_reason, as in the OpenAI API
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Halo\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" dunia\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":42}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "")
	ch, err := p.Stream(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, 0)
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
}

func TestOpenAIStreamFallbackCountsDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 7; i++ {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	ch, err := NewOpenAIProvider(srv.URL, "").Stream(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, 0)
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
