package tooling

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

const (
	imageAPITimeout   = 5 * time.Minute
	maxImageAPIBody   = 96 << 20
	maxImageFromURL   = 32 << 20
	maxImageAPIErrMsg = 300
)

// ImageAPIRequest is what the image generator page may ask for. Values are validated by the caller.
type ImageAPIRequest struct {
	Model        string
	Prompt       string
	N            int
	Size         string
	Quality      string
	Background   string
	OutputFormat string
	Watermark    bool
	PromptExtend bool
}

// ImageAPI talks to an OpenAI-style /images/generations endpoint (JSON or event-stream replies).
type ImageAPI struct {
	URL    string
	APIKey string
}

type imageItem struct {
	B64JSON string `json:"b64_json"`
	URL     string `json:"url"`
}

// Generate returns the decoded images. The API key is only ever sent to the configured URL.
func (a ImageAPI) Generate(ctx context.Context, r ImageAPIRequest) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, imageAPITimeout)
	defer cancel()

	body, _ := json.Marshal(map[string]any{
		"model": r.Model, "prompt": r.Prompt, "n": r.N, "size": r.Size, "quality": r.Quality,
		"background": r.Background, "image_detail": "high", "output_format": r.OutputFormat,
		"watermark": r.Watermark, "prompt_extend": r.PromptExtend,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	if a.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.APIKey)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("tidak bisa menghubungi layanan gambar: %w", redactKey(err, a.APIKey))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("layanan gambar menjawab status %d: %s", resp.StatusCode, apiErrorText(msg))
	}

	items, err := parseImageResponse(io.LimitReader(resp.Body, maxImageAPIBody))
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("layanan gambar tidak mengembalikan gambar")
	}
	out := make([][]byte, 0, len(items))
	for _, it := range items {
		data, err := it.bytes(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, data)
	}
	return out, nil
}

func redactKey(err error, key string) error {
	if key == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), key, "***"))
}

// apiErrorText extracts error.message from a JSON error body, or a short plain-text excerpt.
func apiErrorText(body []byte) string {
	var e struct {
		Error any    `json:"error"`
		Msg   string `json:"message"`
	}
	text := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &e) == nil {
		switch v := e.Error.(type) {
		case string:
			text = v
		case map[string]any:
			if m, ok := v["message"].(string); ok {
				text = m
			}
		default:
			if e.Msg != "" {
				text = e.Msg
			}
		}
	}
	r := []rune(text)
	if len(r) > maxImageAPIErrMsg {
		r = r[:maxImageAPIErrMsg]
	}
	return string(r)
}

// parseImageResponse accepts a plain JSON body or a text/event-stream body. Partial (preview) frames are ignored.
func parseImageResponse(r io.Reader) ([]imageItem, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	var items []imageItem
	var raw bytes.Buffer // everything read, to try as plain JSON if no SSE frames were seen
	sawSSE := false
	for {
		line, err := br.ReadString('\n')
		raw.WriteString(line)
		trimmed := strings.TrimSpace(line)
		if payload, ok := strings.CutPrefix(trimmed, "data:"); ok {
			sawSSE = true
			payload = strings.TrimSpace(payload)
			if payload != "" && payload != "[DONE]" {
				got, perr := itemsFromJSON([]byte(payload))
				if perr != nil {
					return nil, perr
				}
				items = append(items, got...)
			}
		}
		if err != nil {
			if err != io.EOF {
				return nil, fmt.Errorf("gagal membaca jawaban layanan gambar: %w", err)
			}
			break
		}
	}
	if !sawSSE {
		return itemsFromJSON(bytes.TrimSpace(raw.Bytes()))
	}
	return items, nil
}

// itemsFromJSON reads {"data":[...]}, a single {"b64_json"|"url"} object or an {"error":...} object.
func itemsFromJSON(b []byte) ([]imageItem, error) {
	var v struct {
		Type    string      `json:"type"`
		Data    []imageItem `json:"data"`
		B64JSON string      `json:"b64_json"`
		URL     string      `json:"url"`
		Error   any         `json:"error"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, errors.New("jawaban layanan gambar tidak dikenali")
	}
	if v.Error != nil {
		return nil, fmt.Errorf("layanan gambar: %s", apiErrorText(b))
	}
	if strings.Contains(v.Type, "partial") {
		return nil, nil
	}
	items := v.Data
	if v.B64JSON != "" || v.URL != "" {
		items = append(items, imageItem{B64JSON: v.B64JSON, URL: v.URL})
	}
	return items, nil
}

func (it imageItem) bytes(ctx context.Context) ([]byte, error) {
	if it.B64JSON != "" {
		b64 := it.B64JSON
		if i := strings.Index(b64, "base64,"); i >= 0 { // tolerate data: URLs
			b64 = b64[i+len("base64,"):]
		}
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, errors.New("data gambar dari layanan tidak valid")
		}
		return data, nil
	}
	if it.URL == "" {
		return nil, errors.New("item gambar kosong")
	}
	if !strings.HasPrefix(it.URL, "https://") && !strings.HasPrefix(it.URL, "http://") {
		return nil, errors.New("URL gambar dari layanan tidak valid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, it.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Transport: publicTransport(), Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal mengunduh gambar hasil: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unduhan gambar hasil status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageFromURL+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageFromURL {
		return nil, errors.New("gambar hasil terlalu besar")
	}
	return data, nil
}
