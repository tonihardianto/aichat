package tooling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

func TavilySearch(ctx context.Context, apiKey string, args json.RawMessage) (string, error) {
	if apiKey == "" {
		return "", errors.New("TAVILY_API_KEY belum diatur")
	}
	var in struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
		Topic      string `json:"topic"`
		Days       int    `json:"days"`
	}
	if err := json.Unmarshal(args, &in); err != nil || in.Query == "" {
		return "", errors.New("parameter query wajib diisi")
	}
	if in.MaxResults < 1 || in.MaxResults > 10 {
		in.MaxResults = 5
	}
	payload := map[string]any{"api_key": apiKey, "query": in.Query, "max_results": in.MaxResults, "search_depth": "basic", "include_answer": true}
	if in.Topic == "news" {
		payload["topic"] = "news"
		if in.Days >= 1 && in.Days <= 30 {
			payload["days"] = in.Days
		}
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxResponseBytes {
		return "", errors.New("respons Tavily melebihi 1 MiB")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Tavily mengembalikan status %d", resp.StatusCode)
	}
	return string(data), nil
}
