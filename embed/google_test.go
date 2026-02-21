package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGoogleSuccess(t *testing.T) {
	want := []float64{0.123, -0.456, 0.789}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":embedContent") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") == "" {
			t.Error("expected key query parameter")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"embedding": map[string]any{
				"values": want,
			},
		})
	}))
	defer srv.Close()

	fn := Google(GoogleConfig{
		URL:    srv.URL,
		APIKey: "test-key",
		Model:  "gemini-embedding-001",
	})

	got, err := fn(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Google() returned error: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("embedding length = %d, want %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != float32(v) {
			t.Errorf("embedding[%d] = %v, want %v", i, got[i], float32(v))
		}
	}
}

func TestGoogleAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"message": "API key not valid"}}`))
	}))
	defer srv.Close()

	fn := Google(GoogleConfig{
		URL:    srv.URL,
		APIKey: "bad-key",
	})

	_, err := fn(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "google error (status 401)") {
		t.Errorf("error message = %q, want to contain %q", err.Error(), "google error (status 401)")
	}
}

func TestGoogleDefaultConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify default model is used in the path
		if !strings.Contains(r.URL.Path, "gemini-embedding-001") {
			t.Errorf("expected default model in path, got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"embedding": map[string]any{
				"values": []float64{0.1, 0.2},
			},
		})
	}))
	defer srv.Close()

	// Only set URL, leave Model empty to test defaults
	fn := Google(GoogleConfig{
		URL: srv.URL,
	})

	_, err := fn(context.Background(), "test")
	if err != nil {
		t.Fatalf("Google() with defaults returned error: %v", err)
	}
}
