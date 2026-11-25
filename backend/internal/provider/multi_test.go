package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func fakeServer(t *testing.T, models []string, reply string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			items := ""
			for i, m := range models {
				if i > 0 {
					items += ","
				}
				items += fmt.Sprintf(`{"id":%q}`, m)
			}
			fmt.Fprintf(w, `{"data":[%s]}`, items)
			return
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", reply)
	}))
}

func TestMultiListModelsPrefixesExtraProviders(t *testing.T) {
	primary := fakeServer(t, []string{"llama3.2"}, "")
	extra := fakeServer(t, []string{"qwen3", "gemma3"}, "")
	defer primary.Close()
	defer extra.Close()

	m := NewMultiProvider(NewOpenAIProvider(primary.URL, ""), []ExtraProvider{
		{Name: "mac", Provider: NewOpenAIProvider(extra.URL, ""), Models: []string{"qwen3"}},
	})
	got, err := m.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"llama3.2", "mac::qwen3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
}

func TestMultiStreamRoutesByPrefix(t *testing.T) {
	primary := fakeServer(t, nil, "dari-primary")
	extra := fakeServer(t, nil, "dari-extra")
	defer primary.Close()
	defer extra.Close()

	m := NewMultiProvider(NewOpenAIProvider(primary.URL, ""), []ExtraProvider{
		{Name: "mac", Provider: NewOpenAIProvider(extra.URL, "")},
	})
	for model, want := range map[string]string{
		"llama3.2":       "dari-primary",
		"mac::qwen3":     "dari-extra",
		"unknown::model": "dari-primary", // prefix tidak dikenal -> primary
	} {
		ch, err := m.Stream(context.Background(), model, []ChatMessage{{Role: "user", Content: "hi"}}, 0, false)
		if err != nil {
			t.Fatal(err)
		}
		var text strings.Builder
		for c := range ch {
			text.WriteString(c.Delta)
		}
		if text.String() != want {
			t.Fatalf("model %q -> %q, want %q", model, text.String(), want)
		}
	}
}

func TestMultiListModelsSkipsFailingExtra(t *testing.T) {
	primary := fakeServer(t, []string{"llama3.2"}, "")
	defer primary.Close()
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()

	m := NewMultiProvider(NewOpenAIProvider(primary.URL, ""), []ExtraProvider{
		{Name: "mati", Provider: NewOpenAIProvider(dead.URL, "")},
	})
	got, err := m.ListModels(context.Background())
	if err != nil || !reflect.DeepEqual(got, []string{"llama3.2"}) {
		t.Fatalf("got %v, %v", got, err)
	}
}
