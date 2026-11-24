package tooling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	comfyPollInterval = time.Second
	comfyTimeout      = 20 * time.Minute
	maxImageBytes     = 32 << 20
)

// ImageDef is the schema offered to the LLM for the generate_image tool.
var ImageDef = Def{
	Name:        "generate_image",
	Description: "Generate an image from a text description using the user's local ComfyUI (Qwen-Image). Write the prompt in English, detailed and descriptive (subject, setting, lighting, style). Takes up to a minute. The result is a markdown image line: copy it verbatim into your reply so the user can see the image.",
	Parameters:  json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string","description":"Detailed English description of the image."},"negative_prompt":{"type":"string","description":"Things to avoid. Optional."},"width":{"type":"integer","minimum":512,"maximum":1536,"description":"Default 1024."},"height":{"type":"integer","minimum":512,"maximum":1536,"description":"Default 1024."},"seed":{"type":"integer","description":"Optional, for reproducible results."}},"required":["prompt"]}`),
}

type imageArgs struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Seed           *int64 `json:"seed"`
}

func clampDim(v int) int {
	if v <= 0 {
		return 1024
	}
	v = max(512, min(1536, v))
	return v / 32 * 32
}

// ComfyWorkflow builds the API-format graph (Qwen-Image 2.1 GGUF + 8-step turbo LoRA).
func ComfyWorkflow(a imageArgs, seed int64) map[string]any {
	w, h := clampDim(a.Width), clampDim(a.Height)
	return map[string]any{
		"1":  map[string]any{"class_type": "UnetLoaderGGUF", "inputs": map[string]any{"unet_name": "qwen-image-2.1-step90000-Q4_K_M.gguf"}},
		"2":  map[string]any{"class_type": "CLIPLoaderGGUF", "inputs": map[string]any{"clip_name": "Qwen3VL-8B-Instruct-Q4_K_M.gguf", "type": "qwen_image"}},
		"3":  map[string]any{"class_type": "VAELoader", "inputs": map[string]any{"vae_name": "qwen_image_2.1_vae_bf16.safetensors"}},
		"4":  map[string]any{"class_type": "LoraLoaderBypassModelOnly", "inputs": map[string]any{"model": []any{"1", 0}, "lora_name": "chatrlabs-omni-turbo-8step-fp32.safetensors", "strength_model": 1}},
		"5":  map[string]any{"class_type": "QwenImage21Cache", "inputs": map[string]any{"model": []any{"4", 0}, "device": "auto", "dtype": "default"}},
		"6":  map[string]any{"class_type": "ModelSamplingFlux", "inputs": map[string]any{"model": []any{"5", 0}, "max_shift": 0.6935483870967742, "base_shift": 0.5, "width": 768, "height": 768}},
		"7":  map[string]any{"class_type": "TextEncodeQwenImage21", "inputs": map[string]any{"clip": []any{"2", 0}, "prompt": a.Prompt, "negative_prompt": a.NegativePrompt, "resolution": 1024}},
		"8":  map[string]any{"class_type": "EmptyLatentImage", "inputs": map[string]any{"width": w, "height": h, "batch_size": 1}},
		"9":  map[string]any{"class_type": "KSampler", "inputs": map[string]any{"model": []any{"6", 0}, "positive": []any{"7", 0}, "negative": []any{"7", 1}, "latent_image": []any{"8", 0}, "seed": seed, "steps": 8, "cfg": 1, "sampler_name": "euler", "scheduler": "simple", "denoise": 1}},
		"10": map[string]any{"class_type": "VAEDecode", "inputs": map[string]any{"samples": []any{"9", 0}, "vae": []any{"3", 0}}},
		"11": map[string]any{"class_type": "SaveImage", "inputs": map[string]any{"images": []any{"10", 0}, "filename_prefix": "aichat/gen"}},
	}
}

// GenerateImage runs the text-to-image workflow on a local ComfyUI server and returns PNG bytes.
func GenerateImage(ctx context.Context, baseURL string, raw json.RawMessage) ([]byte, error) {
	a, seed, err := parseImageArgs(raw)
	if err != nil {
		return nil, err
	}
	return runWorkflow(ctx, baseURL, ComfyWorkflow(a, seed), nil)
}

// EditImage runs the reference-image workflow: the output follows the reference's aspect ratio (~1 MP).
func EditImage(ctx context.Context, baseURL string, raw json.RawMessage, ref []byte) ([]byte, error) {
	if len(ref) == 0 {
		return nil, errors.New("tidak ada gambar referensi")
	}
	a, seed, err := parseImageArgs(raw)
	if err != nil {
		return nil, err
	}
	return runWorkflow(ctx, baseURL, ComfyReferenceWorkflow(a, seed), ref)
}

func parseImageArgs(raw json.RawMessage) (imageArgs, int64, error) {
	var a imageArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return a, 0, errors.New("argumen tidak valid")
	}
	a.Prompt = strings.TrimSpace(a.Prompt)
	if a.Prompt == "" {
		return a, 0, errors.New("prompt wajib diisi")
	}
	seed := rand.Int64N(1 << 50)
	if a.Seed != nil && *a.Seed >= 0 {
		seed = *a.Seed
	}
	return a, seed, nil
}

// runWorkflow queues the graph, waits for it to finish and returns the first output image.
// When refImage is set it is uploaded first and node "1" (LoadImage) is pointed at it.
func runWorkflow(ctx context.Context, baseURL string, wf map[string]any, refImage []byte) ([]byte, error) {
	base := strings.TrimRight(baseURL, "/")
	ctx, cancel := context.WithTimeout(ctx, comfyTimeout)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second}

	if refImage != nil {
		name, err := comfyUpload(ctx, client, base, refImage)
		if err != nil {
			return nil, err
		}
		wf["1"].(map[string]any)["inputs"].(map[string]any)["image"] = name
	}
	body, _ := json.Marshal(map[string]any{"prompt": wf, "client_id": uuid.NewString()})
	var queued struct {
		PromptID string `json:"prompt_id"`
	}
	if err := comfyDo(ctx, client, http.MethodPost, base+"/prompt", body, &queued); err != nil {
		return nil, fmt.Errorf("ComfyUI menolak workflow: %w", err)
	}
	if queued.PromptID == "" {
		return nil, errors.New("ComfyUI tidak mengembalikan prompt_id")
	}

	type imgRef struct {
		Filename  string `json:"filename"`
		Subfolder string `json:"subfolder"`
		Type      string `json:"type"`
	}
	var ref *imgRef
	for ref == nil {
		select {
		case <-ctx.Done():
			return nil, errors.New("pembuatan gambar melebihi batas waktu")
		case <-time.After(comfyPollInterval):
		}
		var hist map[string]struct {
			Status struct {
				StatusStr string `json:"status_str"`
			} `json:"status"`
			Outputs map[string]struct {
				Images []imgRef `json:"images"`
			} `json:"outputs"`
		}
		if err := comfyDo(ctx, client, http.MethodGet, base+"/history/"+url.PathEscape(queued.PromptID), nil, &hist); err != nil {
			return nil, err
		}
		entry, ok := hist[queued.PromptID]
		if !ok {
			continue
		}
		if entry.Status.StatusStr == "error" {
			return nil, errors.New("ComfyUI gagal menjalankan workflow (cek log ComfyUI)")
		}
		for _, out := range entry.Outputs {
			if len(out.Images) > 0 {
				r := out.Images[0]
				ref = &r
				break
			}
		}
		if ref == nil && entry.Status.StatusStr == "success" {
			return nil, errors.New("workflow selesai tanpa gambar")
		}
	}

	q := url.Values{"filename": {ref.Filename}, "subfolder": {ref.Subfolder}, "type": {ref.Type}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/view?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil gambar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ComfyUI /view status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, errors.New("gambar terlalu besar")
	}
	return data, nil
}

func comfyDo(ctx context.Context, client *http.Client, method, u string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("tidak bisa menghubungi ComfyUI: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return fmt.Errorf("status %d: %s", resp.StatusCode, msg)
	}
	return json.Unmarshal(data, out)
}

func comfyUpload(ctx context.Context, client *http.Client, base string, data []byte) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("image", "aichat-ref-"+uuid.NewString()+".png")
	if err != nil {
		return "", err
	}
	_, _ = fw.Write(data)
	_ = mw.WriteField("type", "input")
	_ = mw.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/upload/image", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gagal mengunggah gambar referensi: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		Name string `json:"name"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out) != nil || out.Name == "" {
		return "", fmt.Errorf("ComfyUI menolak gambar referensi (status %d)", resp.StatusCode)
	}
	return out.Name, nil
}

// ComfyReferenceWorkflow is the image-from-reference graph. Node "1" is the LoadImage whose file name is filled in at run time.
func ComfyReferenceWorkflow(a imageArgs, seed int64) map[string]any {
	return map[string]any{
		"1":  map[string]any{"class_type": "LoadImage", "inputs": map[string]any{"image": ""}},
		"2":  map[string]any{"class_type": "UnetLoaderGGUF", "inputs": map[string]any{"unet_name": "qwen-image-2.1-step90000-Q4_K_M.gguf"}},
		"3":  map[string]any{"class_type": "CLIPLoaderGGUF", "inputs": map[string]any{"clip_name": "Qwen3VL-8B-Instruct-Q4_K_M.gguf", "type": "qwen_image"}},
		"4":  map[string]any{"class_type": "VAELoader", "inputs": map[string]any{"vae_name": "qwen_image_2.1_vae_bf16.safetensors"}},
		"5":  map[string]any{"class_type": "LoraLoaderModelOnly", "inputs": map[string]any{"model": []any{"2", 0}, "lora_name": "chatrlabs-omni-turbo-8step-fp32.safetensors", "strength_model": 1}},
		"6":  map[string]any{"class_type": "QwenImage21Cache", "inputs": map[string]any{"model": []any{"5", 0}, "device": "auto", "dtype": "default"}},
		"7":  map[string]any{"class_type": "GetImageSize", "inputs": map[string]any{"image": []any{"1", 0}}},
		"8":  map[string]any{"class_type": "ComfyMathExpression", "inputs": map[string]any{"expression": "max(256, 32 * round(sqrt(1048576 * min(4, max(0.25, a / b))) / 32))", "values.a": []any{"7", 0}, "values.b": []any{"7", 1}}},
		"9":  map[string]any{"class_type": "ComfyMathExpression", "inputs": map[string]any{"expression": "max(256, 32 * round(sqrt(1048576 / min(4, max(0.25, a / b))) / 32))", "values.a": []any{"7", 0}, "values.b": []any{"7", 1}}},
		"10": map[string]any{"class_type": "ModelSamplingFlux", "inputs": map[string]any{"model": []any{"6", 0}, "max_shift": 0.6935483870967742, "base_shift": 0.5, "width": []any{"8", 1}, "height": []any{"9", 1}}},
		"11": map[string]any{"class_type": "TextEncodeQwenImage21", "inputs": map[string]any{"clip": []any{"3", 0}, "images.image_1": []any{"1", 0}, "vae": []any{"4", 0}, "prompt": a.Prompt, "negative_prompt": a.NegativePrompt, "resolution": 768}},
		"12": map[string]any{"class_type": "EmptyLatentImage", "inputs": map[string]any{"width": []any{"8", 1}, "height": []any{"9", 1}, "batch_size": 1}},
		"13": map[string]any{"class_type": "KSampler", "inputs": map[string]any{"model": []any{"10", 0}, "positive": []any{"11", 0}, "negative": []any{"11", 1}, "latent_image": []any{"12", 0}, "seed": seed, "steps": 8, "cfg": 1, "sampler_name": "euler", "scheduler": "simple", "denoise": 1}},
		"14": map[string]any{"class_type": "VAEDecode", "inputs": map[string]any{"samples": []any{"13", 0}, "vae": []any{"4", 0}}},
		"15": map[string]any{"class_type": "SaveImage", "inputs": map[string]any{"images": []any{"14", 0}, "filename_prefix": "aichat/ref"}},
	}
}

// EditImageDef is offered only when the conversation contains an image the user attached.
var EditImageDef = Def{
	Name:        "edit_image",
	Description: "Generate a new image using the image the user attached most recently as a reference (e.g. keep the same subject, face, outfit or scene, but follow the new instructions). The output keeps the reference's aspect ratio. Write the prompt in English and say what to keep and what to change. The result is a markdown image line: copy it verbatim into your reply.",
	Parameters:  json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string","description":"English description of the desired result, referring to the reference image."},"negative_prompt":{"type":"string","description":"Optional."},"seed":{"type":"integer","description":"Optional."}},"required":["prompt"]}`),
}
