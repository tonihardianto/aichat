package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ChatMessage struct {
	Role    string
	Content string
	Images  []ImageData
}

type ImageData struct {
	ContentType string
	Data        []byte
}

type StreamChunk struct {
	Delta        string
	Reasoning    string // thinking tokens, streamed separately from the answer
	InputTokens  int
	OutputTokens int
	TokensPerSec float64
	FinishReason string
	Error        error
}

type Provider interface {
	ListModels(ctx context.Context) ([]string, error)
	Stream(ctx context.Context, model string, messages []ChatMessage, maxTokens int, think bool) (<-chan StreamChunk, error)
}

type FunctionTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolExecutor func(context.Context, string, json.RawMessage) (string, error)

type ToolCapable interface {
	StreamWithTools(ctx context.Context, model string, messages []ChatMessage, maxTokens int, think bool, tools []FunctionTool, execute ToolExecutor) (<-chan StreamChunk, error)
}

type OpenAIProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewOpenAIProvider(baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 0, // Streaming requests manage their own context timeout
		},
	}
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]string, error) {
	reqURL := fmt.Sprintf("%s/models", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("models endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode models: %w", err)
	}

	var modelIDs []string
	for _, m := range parsed.Data {
		if m.ID != "" {
			modelIDs = append(modelIDs, m.ID)
		}
	}
	return modelIDs, nil
}

type openAIMessageContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"` // string or []openAIMessageContentPart
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIChatRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	Stream    bool            `json:"stream"`
	Think     bool            `json:"think"`
	MaxTokens int             `json:"max_tokens,omitempty"`

	StreamOptions *openAIStreamOptions `json:"stream_options,omitempty"`

	// Ollama's OpenAI-compatible endpoint ignores "think"; it only honors
	// reasoning_effort ("none" disables thinking).
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Tools           []struct {
		Type     string       `json:"type"`
		Function FunctionTool `json:"function"`
	} `json:"tools,omitempty"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIChatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
			// Thinking models stream their reasoning in a separate field. Those
			// tokens count toward completion_tokens, so they must count for timing.
			Reasoning        string `json:"reasoning"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// maxToolRounds bounds how many times the model may call tools before it must answer.
const maxToolRounds = 4

// StreamWithTools streams the answer in a single request that also carries the tool definitions: text reaches the
// caller as it is generated, and only when the model asks for tools are they executed and another round started.
func (p *OpenAIProvider) StreamWithTools(ctx context.Context, model string, messages []ChatMessage, maxTokens int, think bool, tools []FunctionTool, execute ToolExecutor) (<-chan StreamChunk, error) {
	if len(tools) == 0 || execute == nil {
		return p.Stream(ctx, model, messages, maxTokens, think)
	}
	return p.streamRounds(ctx, model, p.requestMessages(messages), maxTokens, think, tools, execute)
}

func (p *OpenAIProvider) requestMessages(messages []ChatMessage) []openAIMessage {
	var out []openAIMessage
	for _, m := range messages {
		if len(m.Images) == 0 {
			out = append(out, openAIMessage{Role: m.Role, Content: m.Content})
			continue
		}
		var parts []openAIMessageContentPart
		if m.Content != "" {
			parts = append(parts, openAIMessageContentPart{Type: "text", Text: m.Content})
		}
		for _, img := range m.Images {
			parts = append(parts, openAIMessageContentPart{Type: "image_url", ImageURL: &struct {
				URL string `json:"url"`
			}{URL: fmt.Sprintf("data:%s;base64,%s", img.ContentType, base64.StdEncoding.EncodeToString(img.Data))}})
		}
		out = append(out, openAIMessage{Role: m.Role, Content: parts})
	}
	return out
}

func (p *OpenAIProvider) Stream(ctx context.Context, model string, messages []ChatMessage, maxTokens int, think bool) (<-chan StreamChunk, error) {
	return p.streamOpenAIMessages(ctx, model, p.requestMessages(messages), maxTokens, think)
}

func (p *OpenAIProvider) streamOpenAIMessages(ctx context.Context, model string, reqMessages []openAIMessage, maxTokens int, think bool) (<-chan StreamChunk, error) {
	return p.streamRounds(ctx, model, reqMessages, maxTokens, think, nil, nil)
}

// openRound sends one streaming chat completion request. tools may be nil.
func (p *OpenAIProvider) openRound(ctx context.Context, model string, reqMessages []openAIMessage, maxTokens int, think bool, tools []FunctionTool) (*http.Response, error) {
	chatReq := openAIChatRequest{
		Model:         model,
		Messages:      reqMessages,
		Stream:        true,
		Think:         think,
		MaxTokens:     maxTokens,
		StreamOptions: &openAIStreamOptions{IncludeUsage: true},
	}
	if !think {
		chatReq.ReasoningEffort = "none"
	}
	for _, tool := range tools {
		chatReq.Tools = append(chatReq.Tools, struct {
			Type     string       `json:"type"`
			Function FunctionTool `json:"function"`
		}{Type: "function", Function: tool})
	}
	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/chat/completions", p.baseURL), bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call chat completions: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("chat completions returned status %d: %s", resp.StatusCode, string(body))
	}
	return resp, nil
}

// roundResult is what one streamed completion produced.
type roundResult struct {
	content      string
	toolCalls    []openAIToolCall
	finishReason string
	inputTokens  int
	usageTokens  int // authoritative completion_tokens reported by the server
	deltaCount   int // fallback estimate: number of streamed chunks
	firstToken   time.Time
}

// readRound forwards text and reasoning to out while it arrives and collects tool calls. It always closes resp.Body.
func readRound(ctx context.Context, resp *http.Response, out chan<- StreamChunk) (roundResult, error) {
	defer resp.Body.Close()
	var res roundResult
	var content strings.Builder
	calls := map[int]*openAIToolCall{}
	var order []int

	scanner := bufio.NewScanner(resp.Body)
	// Set scan buffer to support large tokens/lines
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk openAIChatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			res.inputTokens = chunk.Usage.PromptTokens
			res.usageTokens = chunk.Usage.CompletionTokens
		}
		for _, choice := range chunk.Choices {
			d := choice.Delta
			if res.firstToken.IsZero() && (d.Content != "" || d.Reasoning != "" || d.ReasoningContent != "") {
				res.firstToken = time.Now()
			}
			if r := d.Reasoning + d.ReasoningContent; r != "" {
				res.deltaCount++ // fallback estimate must include reasoning tokens too
				out <- StreamChunk{Reasoning: r}
			}
			if d.Content != "" {
				res.deltaCount++
				content.WriteString(d.Content)
				out <- StreamChunk{Delta: d.Content}
			}
			for _, tc := range d.ToolCalls {
				c, ok := calls[tc.Index]
				if !ok {
					c = &openAIToolCall{Type: "function"}
					calls[tc.Index] = c
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					c.ID = tc.ID
				}
				// Servers either send a whole call at once or fragment name/arguments across chunks.
				c.Function.Name += tc.Function.Name
				c.Function.Arguments += tc.Function.Arguments
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				res.finishReason = *choice.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return res, err
	}
	res.content = content.String()
	for _, i := range order {
		c := calls[i]
		if c.Function.Name == "" {
			continue
		}
		if c.ID == "" {
			c.ID = fmt.Sprintf("call_%d", i)
		}
		if c.Function.Arguments == "" {
			c.Function.Arguments = "{}"
		}
		res.toolCalls = append(res.toolCalls, *c)
	}
	return res, nil
}

// streamRounds streams a completion. With tools, a round that ends in tool calls is followed by the tool results and
// another round, so the user sees text as soon as the model writes it and nothing is generated twice.
func (p *OpenAIProvider) streamRounds(ctx context.Context, model string, reqMessages []openAIMessage, maxTokens int, think bool, tools []FunctionTool, execute ToolExecutor) (<-chan StreamChunk, error) {
	resp, err := p.openRound(ctx, model, reqMessages, maxTokens, think, tools)
	if err != nil {
		return nil, err
	}
	out := make(chan StreamChunk, 32)

	go func() {
		defer close(out)
		var (
			total      roundResult // totals across rounds
			firstToken time.Time
			messages   = reqMessages
		)
		for round := 0; ; round++ {
			res, err := readRound(ctx, resp, out)
			if err != nil {
				out <- StreamChunk{Error: err}
				return
			}
			if firstToken.IsZero() {
				firstToken = res.firstToken
			}
			total.inputTokens = res.inputTokens
			total.usageTokens += res.usageTokens
			total.deltaCount += res.deltaCount
			total.finishReason = res.finishReason

			if len(res.toolCalls) == 0 || execute == nil || round >= maxToolRounds {
				p.finishStream(out, total, firstToken)
				return
			}

			messages = append(messages, openAIMessage{Role: "assistant", Content: res.content, ToolCalls: res.toolCalls})
			for _, call := range res.toolCalls {
				result, err := execute(ctx, call.Function.Name, json.RawMessage(call.Function.Arguments))
				if err != nil {
					result = "Tool error: " + err.Error()
				}
				messages = append(messages, openAIMessage{Role: "tool", ToolCallID: call.ID, Content: result})
			}
			// After the last allowed round the model must answer without tools.
			roundTools := tools
			if round+1 >= maxToolRounds {
				roundTools = nil
			}
			resp, err = p.openRound(ctx, model, messages, maxTokens, think, roundTools)
			if err != nil {
				out <- StreamChunk{Error: err}
				return
			}
		}
	}()

	return out, nil
}

// finishStream emits the terminal chunk with token counts and speed.
func (p *OpenAIProvider) finishStream(out chan<- StreamChunk, total roundResult, firstToken time.Time) {
	reason := total.finishReason
	if reason == "" || reason == "tool_calls" {
		reason = "stop"
	}
	outputTokens := total.usageTokens
	if outputTokens == 0 {
		outputTokens = total.deltaCount
	}
	tokensPerSec := 0.0
	if !firstToken.IsZero() && outputTokens > 0 {
		if d := time.Since(firstToken).Seconds(); d > 0 {
			tokensPerSec = float64(outputTokens) / d
		}
	}
	out <- StreamChunk{FinishReason: reason, InputTokens: total.inputTokens, OutputTokens: outputTokens, TokensPerSec: tokensPerSec}
}

// FakeProvider for tests
type FakeProvider struct {
	Models    []string
	Responses []string
	// FinishReason is what the stream reports at the end; empty means "stop". Use "length" to mimic a full context.
	FinishReason string
}

func NewFakeProvider(models []string, responses []string) *FakeProvider {
	return &FakeProvider{
		Models:    models,
		Responses: responses,
	}
}

func (f *FakeProvider) ListModels(ctx context.Context) ([]string, error) {
	return f.Models, nil
}

func (f *FakeProvider) Stream(ctx context.Context, model string, messages []ChatMessage, maxTokens int, think bool) (<-chan StreamChunk, error) {
	out := make(chan StreamChunk, 16)
	go func() {
		defer close(out)
		resp := "Ini adalah jawaban tiruan dari fake provider."
		if len(f.Responses) > 0 {
			resp = f.Responses[0]
		}

		words := strings.Split(resp, " ")
		for i, w := range words {
			select {
			case <-ctx.Done():
				out <- StreamChunk{Error: ctx.Err()}
				return
			case <-time.After(10 * time.Millisecond):
			}
			space := " "
			if i == len(words)-1 {
				space = ""
			}
			out <- StreamChunk{
				Delta: w + space,
			}
		}

		reason := f.FinishReason
		if reason == "" {
			reason = "stop"
		}
		out <- StreamChunk{
			FinishReason: reason,
			InputTokens:  15,
			OutputTokens: len(words),
			TokensPerSec: 25.0,
		}
	}()
	return out, nil
}
