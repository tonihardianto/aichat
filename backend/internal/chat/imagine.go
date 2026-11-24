package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"chat/backend/internal/provider"
	"chat/backend/internal/repository"
	"chat/backend/internal/tooling"

	"github.com/google/uuid"
)

var imagineSize = regexp.MustCompile(`^(\d{3,4})[xX×](\d{3,4})$`)

type imagineCmd struct {
	Prompt, Negative string
	Width, Height    int
}

// parseImagine recognises "/imagine [WxH] <prompt> [--no <negative prompt>]".
func parseImagine(content string) (imagineCmd, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(content), "/imagine")
	if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\n' && rest[0] != '\t') {
		return imagineCmd{}, false
	}
	cmd := imagineCmd{}
	rest = strings.TrimSpace(rest)
	if first, tail, _ := strings.Cut(rest, " "); imagineSize.MatchString(first) {
		m := imagineSize.FindStringSubmatch(first)
		cmd.Width, _ = strconv.Atoi(m[1])
		cmd.Height, _ = strconv.Atoi(m[2])
		rest = strings.TrimSpace(tail)
	}
	cmd.Prompt, cmd.Negative, _ = strings.Cut(rest, "--no ")
	cmd.Prompt, cmd.Negative = strings.TrimSpace(cmd.Prompt), strings.TrimSpace(cmd.Negative)
	return cmd, true
}

// startChunks sends /imagine messages straight to ComfyUI and everything else to the LLM.
func (s *Service) startChunks(ctx context.Context, userID, model, content string, messages []provider.ChatMessage, attachments []repository.Attachment, maxTokens int, think bool) (<-chan provider.StreamChunk, error) {
	if cmd, ok := parseImagine(content); ok && s.cfg.ComfyUIURL != "" {
		return s.imagineStream(ctx, userID, cmd, attachments), nil
	}
	pending := make(chan string, 8)
	chunks, err := s.streamWithUserExtensions(ctx, userID, model, messages, maxTokens, think, func(md string) {
		select {
		case pending <- md:
		default:
		}
	})
	if err != nil {
		return nil, err
	}
	// Images produced by tools are injected into the stream here, ahead of the next LLM chunk.
	out := make(chan provider.StreamChunk)
	go func() {
		defer close(out)
		for c := range chunks {
			for drained := false; !drained; {
				select {
				case md := <-pending:
					select {
					case out <- provider.StreamChunk{Delta: md + "\n\n"}:
					case <-ctx.Done():
						return
					}
				default:
					drained = true
				}
			}
			select {
			case out <- c:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (s *Service) imagineStream(ctx context.Context, userID string, cmd imagineCmd, attachments []repository.Attachment) <-chan provider.StreamChunk {
	ch := make(chan provider.StreamChunk, 4)
	go func() {
		defer close(ch)
		fail := func(msg string) {
			ch <- provider.StreamChunk{Delta: msg, FinishReason: "stop"}
		}
		if cmd.Prompt == "" {
			fail("Tulis deskripsi gambar setelah perintah. Contoh: `/imagine 768x1024 kucing astronot di bulan --no teks, watermark`. Lampirkan gambar untuk memakainya sebagai referensi.")
			return
		}
		ch <- provider.StreamChunk{Reasoning: "Membuat gambar di ComfyUI. Ini bisa memakan beberapa menit..."}

		args, _ := json.Marshal(map[string]any{"prompt": cmd.Prompt, "negative_prompt": cmd.Negative, "width": cmd.Width, "height": cmd.Height})
		var data []byte
		var err error
		if len(attachments) > 0 {
			var ref []byte
			if rc, _, derr := s.store.Download(ctx, attachments[0].ObjectKey); derr == nil {
				ref, _ = io.ReadAll(rc)
				rc.Close()
			}
			if len(ref) == 0 {
				fail("Gagal membaca gambar referensi.")
				return
			}
			data, err = tooling.EditImage(ctx, s.cfg.ComfyUIURL, args, ref)
		} else {
			data, err = tooling.GenerateImage(ctx, s.cfg.ComfyUIURL, args)
		}
		if ctx.Err() != nil {
			ch <- provider.StreamChunk{Error: ctx.Err()}
			return
		}
		if err != nil {
			s.logger.Error("imagine_failed", "error", err.Error())
			fail("Gagal membuat gambar: " + err.Error())
			return
		}
		name, err := s.storeGeneratedImage(ctx, userID, data)
		if err != nil {
			fail("Gagal menyimpan gambar: " + err.Error())
			return
		}
		ch <- provider.StreamChunk{Delta: fmt.Sprintf("![%s](/api/v1/generated/%s)", altText(cmd.Prompt), name), FinishReason: "stop"}
	}()
	return ch
}

func altText(prompt string) string {
	r := []rune(strings.NewReplacer("[", "(", "]", ")", "\n", " ").Replace(prompt))
	if len(r) > 80 {
		r = r[:80]
	}
	return string(r)
}

func (s *Service) storeGeneratedImage(ctx context.Context, userID string, data []byte) (string, error) {
	name := uuid.New().String() + ".png"
	if err := s.store.Upload(ctx, "users/"+userID+"/generated/"+name, "image/png", bytes.NewReader(data), int64(len(data))); err != nil {
		return "", err
	}
	return name, nil
}

// IsImagineCommand reports whether content is an /imagine command (image generation needs a logged-in user).
func IsImagineCommand(content string) bool {
	_, ok := parseImagine(content)
	return ok
}
