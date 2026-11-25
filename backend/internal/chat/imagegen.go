package chat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"chat/backend/internal/repository"
	"chat/backend/internal/tooling"

	"github.com/google/uuid"
)

// ImageRequestError is a validation problem the user can fix; its text is safe to show.
type ImageRequestError struct{ Msg string }

func (e *ImageRequestError) Error() string { return e.Msg }

var ErrImageGenDisabled = errors.New("generator gambar belum dikonfigurasi")

var imageSizeRe = regexp.MustCompile(`^\d{3,4}x\d{3,4}$`)

const maxImagePromptLen = 4000

type ImageGenRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	N            int    `json:"n"`
	Size         string `json:"size"`
	Quality      string `json:"quality"`
	Background   string `json:"background"`
	OutputFormat string `json:"outputFormat"`
	Watermark    bool   `json:"watermark"`
	PromptExtend bool   `json:"promptExtend"`
}

const (
	MaxImageReferences    = 4
	maxImageReferenceSize = 10 << 20
)

var ErrImageReference = &ImageRequestError{"Gambar referensi harus PNG, JPEG, atau WebP, maksimal 10 MB per gambar"}

type ImageGenConfig struct {
	Enabled       bool     `json:"enabled"`
	Models        []string `json:"models"`
	MaxReferences int      `json:"maxReferences"`
}

// ImageGenConfig reports whether the external image API is configured, with the models users may pick.
func (s *Service) ImageGenConfig() ImageGenConfig {
	models := s.cfg.ImageModels
	if models == nil {
		models = []string{}
	}
	return ImageGenConfig{
		Enabled:       s.cfg.ImageAPIURL != "" && s.cfg.ImageAPIKey != "",
		Models:        models,
		MaxReferences: MaxImageReferences,
	}
}

func validateImageReferences(refs [][]byte) error {
	if len(refs) > MaxImageReferences {
		return &ImageRequestError{fmt.Sprintf("Maksimal %d gambar referensi", MaxImageReferences)}
	}
	for _, data := range refs {
		if len(data) == 0 || len(data) > maxImageReferenceSize {
			return ErrImageReference
		}
		if !oneOf(http.DetectContentType(data), "image/png", "image/jpeg", "image/webp") {
			return ErrImageReference
		}
	}
	return nil
}

func oneOf(v string, allowed ...string) bool { return slices.Contains(allowed, v) }

func (s *Service) validateImageRequest(r *ImageGenRequest) error {
	r.Prompt = strings.TrimSpace(r.Prompt)
	if r.Prompt == "" {
		return &ImageRequestError{"Deskripsi gambar wajib diisi"}
	}
	if len([]rune(r.Prompt)) > maxImagePromptLen {
		return &ImageRequestError{fmt.Sprintf("Deskripsi terlalu panjang (maksimal %d karakter)", maxImagePromptLen)}
	}
	if !slices.Contains(s.cfg.ImageModels, r.Model) {
		return &ImageRequestError{"Model gambar tidak tersedia"}
	}
	if r.N == 0 {
		r.N = 1
	}
	if r.N < 1 || r.N > 4 {
		return &ImageRequestError{"Jumlah gambar harus 1 sampai 4"}
	}
	if r.Size == "" {
		r.Size = "auto"
	}
	if r.Size != "auto" && !imageSizeRe.MatchString(r.Size) {
		return &ImageRequestError{"Ukuran gambar tidak valid"}
	}
	if r.Quality == "" {
		r.Quality = "auto"
	}
	if !oneOf(r.Quality, "auto", "low", "medium", "high") {
		return &ImageRequestError{"Kualitas tidak valid"}
	}
	if r.Background == "" {
		r.Background = "auto"
	}
	if !oneOf(r.Background, "auto", "transparent", "opaque") {
		return &ImageRequestError{"Latar tidak valid"}
	}
	if r.OutputFormat == "" {
		r.OutputFormat = "png"
	}
	if !oneOf(r.OutputFormat, "png", "jpeg", "webp") {
		return &ImageRequestError{"Format output tidak valid"}
	}
	return nil
}

// GenerateImages calls the external image API, stores the results for the user and records them in the gallery.
// refs are optional reference images sent along with the prompt.
func (s *Service) GenerateImages(ctx context.Context, userID string, r ImageGenRequest, refs [][]byte) ([]repository.GeneratedImage, error) {
	if !s.ImageGenConfig().Enabled {
		return nil, ErrImageGenDisabled
	}
	if err := s.validateImageRequest(&r); err != nil {
		return nil, err
	}
	if err := validateImageReferences(refs); err != nil {
		return nil, err
	}
	api := tooling.ImageAPI{URL: s.cfg.ImageAPIURL, APIKey: s.cfg.ImageAPIKey}
	images, err := api.Generate(ctx, tooling.ImageAPIRequest{
		Model: r.Model, Prompt: r.Prompt, N: r.N, Size: r.Size, Quality: r.Quality, Background: r.Background,
		OutputFormat: r.OutputFormat, Watermark: r.Watermark, PromptExtend: r.PromptExtend, References: refs,
	})
	if err != nil {
		return nil, err
	}
	if len(images) > r.N {
		images = images[:r.N]
	}
	now := time.Now().UTC()
	saved := make([]repository.GeneratedImage, 0, len(images))
	fail := func(err error) ([]repository.GeneratedImage, error) {
		for _, img := range saved { // do not leave half a batch behind
			_, _ = s.repo.DeleteGeneratedImage(context.Background(), userID, img.ID)
			_ = s.store.Delete(context.Background(), generatedKey(userID, img.FileName))
		}
		return nil, fmt.Errorf("gagal menyimpan gambar: %w", err)
	}
	for _, data := range images {
		name, err := s.storeGeneratedImage(ctx, userID, data)
		if err != nil {
			return fail(err)
		}
		img := repository.GeneratedImage{ID: uuid.NewString(), UserID: userID, FileName: name, Prompt: r.Prompt, Model: r.Model, CreatedAt: now}
		if err := s.repo.CreateGeneratedImage(ctx, &img); err != nil {
			_ = s.store.Delete(context.Background(), generatedKey(userID, name))
			return fail(err)
		}
		saved = append(saved, img)
	}
	return saved, nil
}

func generatedKey(userID, name string) string { return "users/" + userID + "/generated/" + name }

const (
	defaultGalleryLimit = 48
	maxGalleryLimit     = 100
)

// ListGalleryImages returns the user's generated images, newest first.
func (s *Service) ListGalleryImages(ctx context.Context, userID string, limit, offset int) ([]repository.GeneratedImage, error) {
	if limit <= 0 {
		limit = defaultGalleryLimit
	}
	limit = min(limit, maxGalleryLimit)
	return s.repo.ListGeneratedImages(ctx, userID, limit, max(offset, 0))
}

// DeleteGalleryImage removes one image (record and file). It returns repository.ErrNotFound for someone else's image.
func (s *Service) DeleteGalleryImage(ctx context.Context, userID, id string) error {
	name, err := s.repo.DeleteGeneratedImage(ctx, userID, id)
	if err != nil {
		return err
	}
	s.deleteGeneratedFiles(ctx, userID, name)
	return nil
}

// DeleteAllGalleryImages removes every gallery image of the user and returns how many there were.
func (s *Service) DeleteAllGalleryImages(ctx context.Context, userID string) (int, error) {
	names, err := s.repo.DeleteAllGeneratedImages(ctx, userID)
	if err != nil {
		return 0, err
	}
	s.deleteGeneratedFiles(ctx, userID, names...)
	return len(names), nil
}

func (s *Service) deleteGeneratedFiles(ctx context.Context, userID string, names ...string) {
	for _, n := range names {
		if err := s.store.Delete(context.WithoutCancel(ctx), generatedKey(userID, n)); err != nil {
			s.logger.Error("generated_image_file_delete_failed", "error", err.Error(), "file", n)
		}
	}
}
