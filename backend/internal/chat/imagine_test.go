package chat

import "testing"

func TestParseImagine(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want imagineCmd
	}{
		{"/imagine kucing astronot", true, imagineCmd{Prompt: "kucing astronot"}},
		{"  /imagine 768x1024 a red fox --no text, watermark", true, imagineCmd{Prompt: "a red fox", Negative: "text, watermark", Width: 768, Height: 1024}},
		{"/imagine", true, imagineCmd{}},
		{"/imaginary friend", false, imagineCmd{}},
		{"tolong /imagine sesuatu", false, imagineCmd{}},
	}
	for _, c := range cases {
		got, ok := parseImagine(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%q: got %+v,%v want %+v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestIsImagineCommand(t *testing.T) {
	if !IsImagineCommand(" /imagine kucing") || IsImagineCommand("halo /imagine") {
		t.Fatal("IsImagineCommand mismatch")
	}
}
