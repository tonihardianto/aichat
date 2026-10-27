package storage

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestLocalStorageUploadDownloadDelete(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const key = "users/u1/attachments/a.png"
	if err := store.Upload(context.Background(), key, "image/png", strings.NewReader("image"), 5); err != nil {
		t.Fatal(err)
	}
	body, contentType, err := store.Download(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil || string(data) != "image" || contentType != "image/png" {
		t.Fatalf("got %q, %q, %v", data, contentType, err)
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Download(context.Background(), key); err == nil {
		t.Fatal("download after delete should fail")
	}
}
