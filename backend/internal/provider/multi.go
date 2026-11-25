package provider

import (
	"context"
	"errors"
	"strings"
)

// ModelSeparator memisahkan nama provider tambahan dari nama model asli,
// contoh: "mac::qwen3:8b". Model dari provider utama tidak diberi prefix.
const ModelSeparator = "::"

type ExtraProvider struct {
	Name     string
	Provider Provider
	// Models adalah allowlist opsional. Kosong berarti semua model dari /models.
	Models []string
}

// MultiProvider merutekan model ke provider utama atau provider tambahan
// berdasarkan prefix "nama::model".
type MultiProvider struct {
	primary Provider
	extras  []ExtraProvider
	byName  map[string]ExtraProvider
}

func NewMultiProvider(primary Provider, extras []ExtraProvider) *MultiProvider {
	byName := make(map[string]ExtraProvider, len(extras))
	for _, e := range extras {
		byName[e.Name] = e
	}
	return &MultiProvider{primary: primary, extras: extras, byName: byName}
}

// route mengembalikan provider tujuan dan nama model tanpa prefix.
func (m *MultiProvider) route(model string) (Provider, string) {
	if name, rest, ok := strings.Cut(model, ModelSeparator); ok {
		if e, found := m.byName[name]; found {
			return e.Provider, rest
		}
	}
	return m.primary, model
}

func (m *MultiProvider) ListModels(ctx context.Context) ([]string, error) {
	var out []string
	var errs []error
	primaryModels, err := m.primary.ListModels(ctx)
	if err != nil {
		errs = append(errs, err)
	}
	out = append(out, primaryModels...)

	for _, e := range m.extras {
		models, err := e.Provider.ListModels(ctx)
		if err != nil {
			// provider tambahan yang mati tidak boleh mematikan daftar model lain
			errs = append(errs, err)
			continue
		}
		allowed := make(map[string]bool, len(e.Models))
		for _, id := range e.Models {
			allowed[id] = true
		}
		for _, id := range models {
			if len(allowed) == 0 || allowed[id] {
				out = append(out, e.Name+ModelSeparator+id)
			}
		}
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

func (m *MultiProvider) Stream(ctx context.Context, model string, messages []ChatMessage, maxTokens int, think bool) (<-chan StreamChunk, error) {
	p, name := m.route(model)
	return p.Stream(ctx, name, messages, maxTokens, think)
}

func (m *MultiProvider) StreamWithTools(ctx context.Context, model string, messages []ChatMessage, maxTokens int, think bool, tools []FunctionTool, execute ToolExecutor) (<-chan StreamChunk, error) {
	p, name := m.route(model)
	if capable, ok := p.(ToolCapable); ok {
		return capable.StreamWithTools(ctx, name, messages, maxTokens, think, tools, execute)
	}
	return p.Stream(ctx, name, messages, maxTokens, think)
}
