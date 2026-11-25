package chat_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	if _, err := svc.GenerateImages(context.Background(), "u1", chat.ImageGenRequest{Model: "x", Prompt: "p"}, nil); !errors.Is(err, chat.ErrImageGenDisabled) {
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
		if _, err := svc.GenerateImages(context.Background(), "u1", r, nil); !errors.As(err, &reqErr) {
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

	imgs, err := svc.GenerateImages(context.Background(), "u1", chat.ImageGenRequest{Model: "good", Prompt: "kucing", N: 1}, nil)
	if err != nil || len(imgs) != 1 || !strings.HasSuffix(imgs[0].FileName, ".png") {
		t.Fatalf("got %v, %v (extra image beyond N must be dropped)", imgs, err)
	}
	names := []string{imgs[0].FileName}
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

func TestGenerateImagesWithReferences(t *testing.T) {
	png := generateValidPNG()
	var gotImages []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotImages, _ = body["images"].([]any)
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(png) + `"}]}`))
	}))
	defer srv.Close()
	svc := newImageService(&config.Config{ImageAPIURL: srv.URL, ImageAPIKey: "k", ImageModels: []string{"good"}})
	req := chat.ImageGenRequest{Model: "good", Prompt: "buat versi kartun", N: 1}

	if names, err := svc.GenerateImages(context.Background(), "u1", req, [][]byte{png, png}); err != nil || len(names) != 1 {
		t.Fatalf("names=%v err=%v", names, err)
	}
	if len(gotImages) != 2 || !strings.HasPrefix(gotImages[0].(string), "data:image/png;base64,") {
		t.Fatalf("references must be sent as data URLs in \"images\", got %v", gotImages)
	}

	gotImages = nil
	if _, err := svc.GenerateImages(context.Background(), "u1", req, nil); err != nil || gotImages != nil {
		t.Fatalf("no references must not send \"images\": %v %v", gotImages, err)
	}

	var reqErr *chat.ImageRequestError
	if _, err := svc.GenerateImages(context.Background(), "u1", req, [][]byte{[]byte("bukan gambar")}); !errors.As(err, &reqErr) {
		t.Fatalf("non-image reference must be rejected, got %v", err)
	}
	if _, err := svc.GenerateImages(context.Background(), "u1", req, [][]byte{png, png, png, png, png}); !errors.As(err, &reqErr) {
		t.Fatalf("too many references must be rejected, got %v", err)
	}
}

func TestImageGalleryListAndDelete(t *testing.T) {
	png := generateValidPNG()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(png) + `"}]}`))
	}))
	defer srv.Close()
	svc := newImageService(&config.Config{ImageAPIURL: srv.URL, ImageAPIKey: "k", ImageModels: []string{"good"}})
	ctx := context.Background()
	gen := func(user, prompt string) string {
		imgs, err := svc.GenerateImages(ctx, user, chat.ImageGenRequest{Model: "good", Prompt: prompt, N: 1}, nil)
		if err != nil || len(imgs) != 1 || imgs[0].Prompt != prompt || imgs[0].Model != "good" || imgs[0].ID == "" {
			t.Fatalf("generate %q: %+v %v", prompt, imgs, err)
		}
		return imgs[0].ID
	}
	first, second := gen("u1", "pertama"), gen("u1", "kedua")
	other := gen("u2", "milik user lain")

	list, err := svc.ListGalleryImages(ctx, "u1", 0, 0)
	if err != nil || len(list) != 2 || list[0].Prompt != "kedua" || list[1].Prompt != "pertama" {
		t.Fatalf("u1 gallery must be newest first and private: %+v %v", list, err)
	}
	if page, _ := svc.ListGalleryImages(ctx, "u1", 1, 1); len(page) != 1 || page[0].Prompt != "pertama" {
		t.Fatalf("pagination broken: %+v", page)
	}

	if err := svc.DeleteGalleryImage(ctx, "u1", other); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("deleting another user's image must be ErrNotFound, got %v", err)
	}
	if err := svc.DeleteGalleryImage(ctx, "u1", first); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.GetGeneratedImage(ctx, "u1", list[1].FileName); err == nil {
		t.Fatal("file must be gone after delete")
	}
	if rc, _, err := svc.GetGeneratedImage(ctx, "u1", list[0].FileName); err != nil {
		t.Fatal("other image must stay")
	} else {
		rc.Close()
	}

	if n, err := svc.DeleteAllGalleryImages(ctx, "u1"); err != nil || n != 1 {
		t.Fatalf("delete all: %d %v", n, err)
	}
	_ = second
	if rest, _ := svc.ListGalleryImages(ctx, "u2", 0, 0); len(rest) != 1 {
		t.Fatalf("u2 must be untouched: %+v", rest)
	}
}
