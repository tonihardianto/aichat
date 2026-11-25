package chat_test

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chat/backend/internal/chat"
	"chat/backend/internal/config"
	"chat/backend/internal/observability"
	"chat/backend/internal/provider"
	"chat/backend/internal/repository"
	"chat/backend/internal/storage"
)

func newImageService(cfg *config.Config) *chat.Service {
	return chat.NewService(repository.NewMemoryRepo(), storage.NewMemoryStorage(), provider.NewFakeProvider([]string{"m"}, nil), cfg, observability.NewLogger())
}

func TestGenerateImagesDisabledWithoutConfig(t *testing.T) {
	svc := newImageService(&config.Config{})
	if _, err := svc.GenerateImages(context.Background(), "u1", chat.ImageGenRequest{Model: "x", Prompt: "p"}); !errors.Is(err, chat.ErrImageGenDisabled) {
		t.Fatalf("expected disabled, got %v", err)
	}
}

func TestGenerateImagesValidation(t *testing.T) {
	svc := newImageService(&config.Config{ImageAPIURL: "http://unused.invalid", ImageAPIKey: "k", ImageModels: []string{"good"}})
	bad := []chat.ImageGenRequest{
		{Model: "good", Prompt: "  "},
		{Model: "other", Prompt: "p"},
		{Model: "good", Prompt: "p", N: 9},
		{Model: "good", Prompt: "p", Size: "huge"},
		{Model: "good", Prompt: "p", Quality: "ultra"},
		{Model: "good", Prompt: "p", OutputFormat: "gif"},
		{Model: "good", Prompt: strings.Repeat("a", 4001)},
	}
	for i, r := range bad {
		var reqErr *chat.ImageRequestError
		if _, err := svc.GenerateImages(context.Background(), "u1", r); !errors.As(err, &reqErr) {
			t.Fatalf("case %d: expected ImageRequestError, got %v", i, err)
		}
	}
}

func TestGenerateImagesStoresPerUser(t *testing.T) {
	png := generateValidPNG()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(png) + `"},{"b64_json":"` + base64.StdEncoding.EncodeToString(png) + `"}]}`))
	}))
	defer srv.Close()
	svc := newImageService(&config.Config{ImageAPIURL: srv.URL, ImageAPIKey: "k", ImageModels: []string{"good"}})

	names, err := svc.GenerateImages(context.Background(), "u1", chat.ImageGenRequest{Model: "good", Prompt: "kucing", N: 1})
	if err != nil || len(names) != 1 || !strings.HasSuffix(names[0], ".png") {
		t.Fatalf("got %v, %v (extra image beyond N must be dropped)", names, err)
	}
	rc, ct, err := svc.GetGeneratedImage(context.Background(), "u1", names[0])
	if err != nil || ct != "image/png" {
		t.Fatalf("owner read: %v %q", err, ct)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if len(data) != len(png) {
		t.Fatalf("stored bytes differ")
	}
	if _, _, err := svc.GetGeneratedImage(context.Background(), "u2", names[0]); err == nil {
		t.Fatal("other user must not read the image")
	}
	if _, _, err := svc.GetGeneratedImage(context.Background(), "u1", "../../etc/passwd"); err == nil {
		t.Fatal("path traversal must be rejected")
	}
}
