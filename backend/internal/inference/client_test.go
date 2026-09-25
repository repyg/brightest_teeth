package inference

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/repyg/brightest_teeth/backend/api"
	"github.com/repyg/brightest_teeth/backend/internal/mlapi"
)

func TestGeneratedMLClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/readyz" {
			_ = json.NewEncoder(w).Encode(map[string]string{"model_version": "test-v1"})
			return
		}
		var body mlapi.ExtractJSONRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if string(body.Image) != "image bytes" || body.Bbox.W != 8 {
			t.Error("generated request did not preserve input")
		}
		v := make([]float32, 512)
		v[0] = 1
		_ = json.NewEncoder(w).Encode(api.Feature{Embedding: v, ModelVersion: "test-v1"})
	}))
	defer server.Close()
	client, err := New(server.URL, "test-v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	f, err := client.Extract(context.Background(), []byte("image bytes"), api.BBox{W: 8, H: 8})
	if err != nil || len(f.Embedding) != 512 {
		t.Fatalf("extract: %+v %v", f, err)
	}
	client.version = "other"
	if client.Ready(context.Background()) == nil {
		t.Fatal("version mismatch accepted")
	}
}
