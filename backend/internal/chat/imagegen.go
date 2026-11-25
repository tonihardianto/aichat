package chat

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"chat/backend/internal/tooling"
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

// ImageGenEnabled reports whether the external image API is configured, with the models users may pick.
func (s *Service) ImageGenConfig() (bool, []string) {
	return s.cfg.ImageAPIURL != "" && s.cfg.ImageAPIKey != "", s.cfg.ImageModels
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

// GenerateImages calls the external image API and stores the results for the user. It returns the file names.
func (s *Service) GenerateImages(ctx context.Context, userID string, r ImageGenRequest) ([]string, error) {
	if enabled, _ := s.ImageGenConfig(); !enabled {
		return nil, ErrImageGenDisabled
	}
	if err := s.validateImageRequest(&r); err != nil {
		return nil, err
	}
	api := tooling.ImageAPI{URL: s.cfg.ImageAPIURL, APIKey: s.cfg.ImageAPIKey}
	images, err := api.Generate(ctx, tooling.ImageAPIRequest{
		Model: r.Model, Prompt: r.Prompt, N: r.N, Size: r.Size, Quality: r.Quality, Background: r.Background,
		OutputFormat: r.OutputFormat, Watermark: r.Watermark, PromptExtend: r.PromptExtend,
	})
	if err != nil {
		return nil, err
	}
	if len(images) > r.N {
		images = images[:r.N]
	}
	names := make([]string, 0, len(images))
	for _, data := range images {
		name, err := s.storeGeneratedImage(ctx, userID, data)
		if err != nil {
			return nil, fmt.Errorf("gagal menyimpan gambar: %w", err)
		}
		names = append(names, name)
	}
	return names, nil
}
