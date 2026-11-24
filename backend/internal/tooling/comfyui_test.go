package tooling

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateImageAgainstFakeComfyUI(t *testing.T) {
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/prompt":
			_ = json.NewDecoder(r.Body).Decode(&posted)
			_, _ = w.Write([]byte(`{"prompt_id":"p1"}`))
		case r.URL.Path == "/history/p1":
			_, _ = w.Write([]byte(`{"p1":{"status":{"status_str":"success"},"outputs":{"11":{"images":[{"filename":"a.png","subfolder":"aichat","type":"output"}]}}}}`))
		case r.URL.Path == "/view" && r.URL.Query().Get("filename") == "a.png":
			_, _ = w.Write([]byte("PNGDATA"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	data, err := GenerateImage(context.Background(), srv.URL, json.RawMessage(`{"prompt":"a cat","width":1000,"height":5000}`))
	if err != nil || string(data) != "PNGDATA" {
		t.Fatalf("got %q, %v", data, err)
	}
	wf := posted["prompt"].(map[string]any)
	latent := wf["8"].(map[string]any)["inputs"].(map[string]any)
	if latent["width"].(float64) != 992 || latent["height"].(float64) != 1536 {
		t.Fatalf("dimensions not clamped: %v", latent)
	}
}

func TestGenerateImageRejectsEmptyPrompt(t *testing.T) {
	if _, err := GenerateImage(context.Background(), "http://127.0.0.1:1", json.RawMessage(`{"prompt":"  "}`)); err == nil || !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("expected prompt error, got %v", err)
	}
}
