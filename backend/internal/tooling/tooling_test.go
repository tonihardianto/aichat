package tooling

import "testing"

func TestValidatePublicURLRejectsPrivateTargets(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1/secret",
		"http://localhost/admin",
		"http://[::1]/admin",
		"http://169.254.169.254/latest/meta-data",
	} {
		if err := ValidatePublicURL(raw); err == nil {
			t.Fatalf("ValidatePublicURL(%q) accepted private target", raw)
		}
	}
}

func TestValidatePublicURLAllowsHTTPSPublicHost(t *testing.T) {
	if err := ValidatePublicURL("https://example.com/path?q=1"); err != nil {
		t.Fatalf("ValidatePublicURL() error = %v", err)
	}
}

func TestCalculate(t *testing.T) {
	for expr, want := range map[string]string{"2+3*4": "14", "(1+2)^3": "27", "sqrt(16)-abs(-2)": "2", "10%4": "2", "1/4": "0.25"} {
		got, err := Calculate([]byte(`{"expression":"` + expr + `"}`))
		if err != nil || got != want {
			t.Fatalf("%s = %q, %v; want %s", expr, got, err, want)
		}
	}
	for _, bad := range []string{"1/0", "os.Exit(1)", "\\\"a\\\"", "sqrt(-1)"} {
		if _, err := Calculate([]byte(`{"expression":"` + bad + `"}`)); err == nil {
			t.Fatalf("%s accepted", bad)
		}
	}
}

func TestCleanTextStripsHTML(t *testing.T) {
	got := cleanText(`<html><head><title>x</title></head><body><script>evil()</script><p>Halo &amp; dunia</p></body></html>`)
	if got != "Halo & dunia" {
		t.Fatalf("got %q", got)
	}
}

func TestDateTime(t *testing.T) {
	if _, err := DateTime([]byte(`{"timezone":"Asia/Jakarta"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := DateTime([]byte(`{"timezone":"Nope/Zone"}`)); err == nil {
		t.Fatal("bad timezone accepted")
	}
}
