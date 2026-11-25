package tooling

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestParseImageResponseFormats(t *testing.T) {
	cases := map[string]struct {
		body string
		want []string
	}{
		"json data array": {`{"data":[{"b64_json":"` + b64("A") + `"},{"b64_json":"` + b64("B") + `"}]}`, []string{"A", "B"}},
		"sse ignores partial frames": {
			"event: image_generation.partial_image\ndata: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"" + b64("P") + "\"}\n\n" +
				"event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"b64_json\":\"" + b64("DONE") + "\"}\n\ndata: [DONE]\n\n",
			[]string{"DONE"}},
		"sse with data array": {"data: {\"data\":[{\"b64_json\":\"" + b64("X") + "\"}]}\n\n", []string{"X"}},
	}
	for name, c := range cases {
		items, err := parseImageResponse(strings.NewReader(c.body))
		if err != nil || len(items) != len(c.want) {
			t.Fatalf("%s: got %v, %v", name, items, err)
		}
		for i, it := range items {
			got, err := it.bytes(context.Background())
			if err != nil || string(got) != c.want[i] {
				t.Fatalf("%s[%d]: got %q, %v", name, i, got, err)
			}
		}
	}
}

func TestParseImageResponseErrors(t *testing.T) {
	for name, body := range map[string]string{
		"json error": `{"error":{"message":"quota habis"}}`,
		"sse error":  "event: error\ndata: {\"error\":{\"message\":\"quota habis\"}}\n\n",
		"not json":   "<html>bad gateway</html>",
	} {
		if _, err := parseImageResponse(strings.NewReader(body)); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestImageAPIGenerateSendsExpectedRequest(t *testing.T) {
	var gotAuth, gotAccept string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotAccept = r.Header.Get("Authorization"), r.Header.Get("Accept")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"data\":[{\"b64_json\":\"" + b64("IMG") + "\"}]}\n\n"))
	}))
	defer srv.Close()

	imgs, err := ImageAPI{URL: srv.URL, APIKey: "sekrit"}.Generate(context.Background(), ImageAPIRequest{
		Model: "m", Prompt: "kucing", N: 1, Size: "auto", Quality: "auto", Background: "auto", OutputFormat: "png", Watermark: true, PromptExtend: true,
	})
	if err != nil || len(imgs) != 1 || string(imgs[0]) != "IMG" {
		t.Fatalf("got %v, %v", imgs, err)
	}
	if gotAuth != "Bearer sekrit" || !strings.Contains(gotAccept, "text/event-stream") {
		t.Fatalf("headers: %q %q", gotAuth, gotAccept)
	}
	if gotBody["model"] != "m" || gotBody["prompt"] != "kucing" || gotBody["image_detail"] != "high" || gotBody["watermark"] != true || gotBody["output_format"] != "png" {
		t.Fatalf("body: %v", gotBody)
	}
}

func TestImageAPIGenerateReportsProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"token salah"}}`))
	}))
	defer srv.Close()
	_, err := ImageAPI{URL: srv.URL, APIKey: "sekrit"}.Generate(context.Background(), ImageAPIRequest{Model: "m", Prompt: "x", N: 1})
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "token salah") || strings.Contains(err.Error(), "sekrit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImageItemPrefersBase64ThenFallsBackToURL(t *testing.T) {
	// Documented reply shape: {"data":[{"url":"...","b64_json":"..."}]}
	items, err := parseImageResponse(strings.NewReader(`{"data":[{"url":"http://127.0.0.1:1/never-fetched.png","b64_json":"` + b64("FROM-B64") + `"}]}`))
	if err != nil || len(items) != 1 {
		t.Fatalf("got %v, %v", items, err)
	}
	if got, err := items[0].bytes(context.Background()); err != nil || string(got) != "FROM-B64" {
		t.Fatalf("b64 must win when both are present: %q, %v", got, err)
	}

	// url only (empty b64_json): downloaded; private/loopback targets are refused by the SSRF guard.
	items, _ = parseImageResponse(strings.NewReader(`{"data":[{"url":"http://127.0.0.1:1/x.png","b64_json":""}]}`))
	if _, err := items[0].bytes(context.Background()); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("loopback URL must be blocked, got %v", err)
	}
	if _, err := (imageItem{URL: "ftp://example.com/x.png"}).bytes(context.Background()); err == nil {
		t.Fatal("non-http URL must be rejected")
	}
}
