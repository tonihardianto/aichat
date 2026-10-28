package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type openAICompletionResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
}

func (p *OpenAIProvider) StreamWithTools(ctx context.Context, model string, messages []ChatMessage, maxTokens int, think bool, tools []FunctionTool, execute ToolExecutor) (<-chan StreamChunk, error) {
	if len(tools) == 0 {
		return p.Stream(ctx, model, messages, maxTokens, think)
	}
	requestMessages := p.requestMessages(messages)
	for range 4 {
		toolDefs := make([]struct {
			Type     string       `json:"type"`
			Function FunctionTool `json:"function"`
		}, len(tools))
		for i, tool := range tools {
			toolDefs[i] = struct {
				Type     string       `json:"type"`
				Function FunctionTool `json:"function"`
			}{Type: "function", Function: tool}
		}
		toolReq := openAIChatRequest{Model: model, Messages: requestMessages, Think: think, MaxTokens: maxTokens, Tools: toolDefs}
		if !think {
			toolReq.ReasoningEffort = "none"
		}
		reqBody, err := json.Marshal(toolReq)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/chat/completions", p.baseURL), bytes.NewReader(reqBody))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if p.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to call tools: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("tools returned status %d: %s", resp.StatusCode, data)
		}
		var parsed openAICompletionResponse
		err = json.NewDecoder(resp.Body).Decode(&parsed)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if len(parsed.Choices) == 0 {
			return nil, errors.New("tools returned no choices")
		}
		assistant := parsed.Choices[0].Message
		if len(assistant.ToolCalls) == 0 {
			break
		}
		requestMessages = append(requestMessages, assistant)
		for _, call := range assistant.ToolCalls {
			result, err := execute(ctx, call.Function.Name, json.RawMessage(call.Function.Arguments))
			if err != nil {
				result = "Tool error: " + err.Error()
			}
			requestMessages = append(requestMessages, openAIMessage{Role: "tool", ToolCallID: call.ID, Content: result})
		}
	}
	return p.streamOpenAIMessages(ctx, model, requestMessages, maxTokens, think)
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
	out := make(chan StreamChunk, 32)

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

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/chat/completions", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqBody))
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

	go func() {
		defer resp.Body.Close()
		defer close(out)

		scanner := bufio.NewScanner(resp.Body)
		// Set scan buffer to support large tokens/lines
		buf := make([]byte, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		var firstTokenTime time.Time
		deltaCount := 0 // fallback estimate: number of streamed content chunks
		inputTokens := 0
		usageTokens := 0 // authoritative completion_tokens reported by the server
		finishReason := ""

		// finish emits the terminal chunk. It runs after the stream ends so the
		// trailing usage chunk (sent after finish_reason) is included.
		finish := func() {
			if finishReason == "" {
				finishReason = "stop"
			}
			outputTokens := usageTokens
			if outputTokens == 0 {
				outputTokens = deltaCount
			}
			tokensPerSec := 0.0
			if !firstTokenTime.IsZero() && outputTokens > 0 {
				if d := time.Since(firstTokenTime).Seconds(); d > 0 {
					tokensPerSec = float64(outputTokens) / d
				}
			}
			out <- StreamChunk{
				FinishReason: finishReason,
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				TokensPerSec: tokensPerSec,
			}
		}

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				out <- StreamChunk{Error: ctx.Err()}
				return
			default:
			}

			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}

			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				finish()
				return
			}

			var chunk openAIChatChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if chunk.Usage != nil {
				inputTokens = chunk.Usage.PromptTokens
				usageTokens = chunk.Usage.CompletionTokens
			}

			for _, choice := range chunk.Choices {
				if firstTokenTime.IsZero() && (choice.Delta.Content != "" || choice.Delta.Reasoning != "" || choice.Delta.ReasoningContent != "") {
					firstTokenTime = time.Now()
				}
				if r := choice.Delta.Reasoning + choice.Delta.ReasoningContent; r != "" {
					deltaCount++ // fallback estimate must include reasoning tokens too
					out <- StreamChunk{Reasoning: r}
				}
				if delta := choice.Delta.Content; delta != "" {
					deltaCount++
					out <- StreamChunk{Delta: delta}
				}
				if choice.FinishReason != nil && *choice.FinishReason != "" {
					finishReason = *choice.FinishReason
				}
			}
		}

		if err := scanner.Err(); err != nil {
			out <- StreamChunk{Error: err}
			return
		}
		// Stream closed without [DONE]
		finish()
	}()

	return out, nil
}

// FakeProvider for tests
type FakeProvider struct {
	Models    []string
	Responses []string
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

		out <- StreamChunk{
			FinishReason: "stop",
			InputTokens:  15,
			OutputTokens: len(words),
			TokensPerSec: 25.0,
		}
	}()
	return out, nil
}
