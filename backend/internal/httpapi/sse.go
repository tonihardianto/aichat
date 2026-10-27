package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type httpSSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func newSSEWriter(w http.ResponseWriter) (*httpSSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	return &httpSSEWriter{
		w:       w,
		flusher: flusher,
	}, nil
}

func (s *httpSSEWriter) WriteEvent(event string, data any) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, string(bytes))
	return err
}

func (s *httpSSEWriter) Flush() {
	s.flusher.Flush()
}
