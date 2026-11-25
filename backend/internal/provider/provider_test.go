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

// sse writes one server-sent event with the given JSON payload.
func sse(w http.ResponseWriter, payload string) { fmt.Fprintf(w, "data: %s\n\n", payload) }

func toolDefs() []FunctionTool {
	return []FunctionTool{{Name: "web_search", Parameters: json.RawMessage(`{"type":"object"}`)}}
}

func collect(t *testing.T, ch <-chan StreamChunk) (text string, last StreamChunk) {
	t.Helper()
	var got strings.Builder
	for c := range ch {
		if c.Error != nil {
			t.Fatalf("stream error: %v", c.Error)
		}
		got.WriteString(c.Delta)
		if c.FinishReason != "" {
			last = c
		}
	}
	return got.String(), last
}

func TestStreamWithToolsExecutesToolThenStreams(t *testing.T) {
	requests := 0
	var secondBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests++
		if body["stream"] != true {
			t.Errorf("every request must stream, got %v", body["stream"])
		}
		if requests == 1 {
			sse(w, `{"choices":[{"delta":{"content":"Saya cek dulu. "}}]}`)
			sse(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"web_search","arguments":"{\"query\":\"x\"}"}}]},"finish_reason":"tool_calls"}]}`)
			sse(w, `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		secondBody = body
		sse(w, `{"choices":[{"delta":{"content":"hasil"},"finish_reason":"stop"}]}`)
		sse(w, `{"choices":[],"usage":{"prompt_tokens":30,"completion_tokens":7}}`)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	executed, args := "", ""
	p := NewOpenAIProvider(srv.URL, "")
	ch, err := p.StreamWithTools(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, 0, false, toolDefs(),
		func(_ context.Context, name string, a json.RawMessage) (string, error) {
			executed, args = name, string(a)
			return "data", nil
		})
	if err != nil {
		t.Fatal(err)
	}
	text, last := collect(t, ch)
	if executed != "web_search" || args != `{"query":"x"}` {
		t.Fatalf("tool call wrong: %q %q", executed, args)
	}
	if text != "Saya cek dulu. hasil" {
		t.Fatalf("text before and after the tool must both reach the user, got %q", text)
	}
	if requests != 2 {
		t.Fatalf("want 2 requests (tool round + answer), got %d", requests)
	}
	if last.FinishReason != "stop" || last.OutputTokens != 12 || last.InputTokens != 30 {
		t.Fatalf("totals wrong: %+v", last)
	}
	msgs, _ := secondBody["messages"].([]any)
	if len(msgs) != 3 || msgs[2].(map[string]any)["role"] != "tool" || msgs[2].(map[string]any)["tool_call_id"] != "c1" {
		t.Fatalf("second request must carry the assistant tool call and its result: %v", msgs)
	}
}

func TestStreamWithToolsDoesNotGenerateTwiceWithoutToolCalls(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		sse(w, `{"choices":[{"delta":{"content":"Jawaban "}}]}`)
		sse(w, `{"choices":[{"delta":{"content":"langsung"},"finish_reason":"stop"}]}`)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	p := NewOpenAIProvider(srv.URL, "")
	ch, err := p.StreamWithTools(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, 0, false, toolDefs(),
		func(context.Context, string, json.RawMessage) (string, error) {
			t.Fatal("no tool expected")
			return "", nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := collect(t, ch); text != "Jawaban langsung" || requests != 1 {
		t.Fatalf("text=%q requests=%d (answer must be generated exactly once)", text, requests)
	}
}

func TestStreamWithToolsAssemblesFragmentedToolCalls(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 { // OpenAI style: name first, arguments in pieces, two calls
			sse(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"web_search","arguments":""}},{"index":1,"id":"b","function":{"name":"web_search","arguments":"{\"q\":"}}]}}]}`)
			sse(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"satu\"}"}},{"index":1,"function":{"arguments":"\"dua\"}"}}]},"finish_reason":"tool_calls"}]}`)
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		sse(w, `{"choices":[{"delta":{"content":"selesai"},"finish_reason":"stop"}]}`)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	var seen []string
	p := NewOpenAIProvider(srv.URL, "")
	ch, _ := p.StreamWithTools(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, 0, false, toolDefs(),
		func(_ context.Context, _ string, a json.RawMessage) (string, error) {
			seen = append(seen, string(a))
			return "ok", nil
		})
	if text, _ := collect(t, ch); text != "selesai" || len(seen) != 2 || seen[0] != `{"q":"satu"}` || seen[1] != `{"q":"dua"}` {
		t.Fatalf("text=%q args=%v", text, seen)
	}
}

func TestStreamWithToolsStopsLoopingAndReportsTruncation(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests++
		if _, hasTools := body["tools"]; hasTools {
			sse(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"x","function":{"name":"web_search","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
		} else { // tools withdrawn after the limit: the model must answer, here cut off by the token limit
			sse(w, `{"choices":[{"delta":{"content":"akhir"},"finish_reason":"length"}]}`)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	p := NewOpenAIProvider(srv.URL, "")
	ch, _ := p.StreamWithTools(context.Background(), "m", []ChatMessage{{Role: "user", Content: "hi"}}, 0, false, toolDefs(),
		func(context.Context, string, json.RawMessage) (string, error) { return "", fmt.Errorf("gagal") })
	text, last := collect(t, ch)
	if text != "akhir" || requests != maxToolRounds+1 || last.FinishReason != "length" {
		t.Fatalf("text=%q requests=%d last=%+v", text, requests, last)
	}
}
