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
	InputTokens  int
	OutputTokens int
	TokensPerSec float64
	FinishReason string
	Error        error
}

type Provider interface {
	ListModels(ctx context.Context) ([]string, error)
	Stream(ctx context.Context, model string, messages []ChatMessage, maxTokens int) (<-chan StreamChunk, error)
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
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []openAIMessageContentPart
}

type openAIChatRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	Stream    bool            `json:"stream"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

type openAIChatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *OpenAIProvider) Stream(ctx context.Context, model string, messages []ChatMessage, maxTokens int) (<-chan StreamChunk, error) {
	out := make(chan StreamChunk, 32)

	var reqMessages []openAIMessage
	for _, m := range messages {
		if len(m.Images) == 0 {
			reqMessages = append(reqMessages, openAIMessage{
				Role:    m.Role,
				Content: m.Content,
			})
		} else {
			var parts []openAIMessageContentPart
			if m.Content != "" {
				parts = append(parts, openAIMessageContentPart{
					Type: "text",
					Text: m.Content,
				})
			}
			for _, img := range m.Images {
				b64 := base64.StdEncoding.EncodeToString(img.Data)
				dataURI := fmt.Sprintf("data:%s;base64,%s", img.ContentType, b64)
				parts = append(parts, openAIMessageContentPart{
					Type: "image_url",
					ImageURL: &struct {
						URL string `json:"url"`
					}{
						URL: dataURI,
					},
				})
			}
			reqMessages = append(reqMessages, openAIMessage{
				Role:    m.Role,
				Content: parts,
			})
		}
	}

	chatReq := openAIChatRequest{
		Model:     model,
		Messages:  reqMessages,
		Stream:    true,
		MaxTokens: maxTokens,
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

		startTime := time.Now()
		totalTokens := 0
		inputTokens := 0
		outputTokens := 0

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				out <- StreamChunk{Error: ctx.Err()}
				return
			default:
			}

			line := scanner.Text()
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if !strings.HasPrefix(line, "data:") {
				continue
			}

			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				duration := time.Since(startTime).Seconds()
				tokensPerSec := 0.0
				if duration > 0 && outputTokens > 0 {
					tokensPerSec = float64(outputTokens) / duration
				}
				out <- StreamChunk{
					FinishReason: "stop",
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
					TokensPerSec: tokensPerSec,
				}
				return
			}

			var chunk openAIChatChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if chunk.Usage != nil {
				inputTokens = chunk.Usage.PromptTokens
				outputTokens = chunk.Usage.CompletionTokens
			}

			for _, choice := range chunk.Choices {
				delta := choice.Delta.Content
				if delta != "" {
					totalTokens++
					if outputTokens == 0 {
						outputTokens = totalTokens
					}
					out <- StreamChunk{
						Delta: delta,
					}
				}

				if choice.FinishReason != nil && *choice.FinishReason != "" {
					duration := time.Since(startTime).Seconds()
					tokensPerSec := 0.0
					if duration > 0 && outputTokens > 0 {
						tokensPerSec = float64(outputTokens) / duration
					}
					out <- StreamChunk{
						FinishReason: *choice.FinishReason,
						InputTokens:  inputTokens,
						OutputTokens: outputTokens,
						TokensPerSec: tokensPerSec,
					}
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			out <- StreamChunk{Error: err}
		}
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

func (f *FakeProvider) Stream(ctx context.Context, model string, messages []ChatMessage, maxTokens int) (<-chan StreamChunk, error) {
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
