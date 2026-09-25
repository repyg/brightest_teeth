package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/repyg/brightest_teeth/backend/api"
)

type memory struct {
	mu           sync.Mutex
	photos       map[uuid.UUID]StoredPhoto
	objects      map[string][]byte
	observations map[uuid.UUID]api.Observation
	vectors      map[uuid.UUID][]float32
	failSave     bool
}

func (m *memory) Ready(context.Context) error { return nil }
func (m *memory) SavePhoto(_ context.Context, p StoredPhoto) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failSave {
		return errors.New("database down")
	}
	m.photos[p.Id] = p
	return nil
}
func (m *memory) Photo(_ context.Context, id uuid.UUID) (StoredPhoto, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.photos[id]
	if !ok {
		return p, ErrNotFound
	}
	return p, nil
}
func (m *memory) SaveObservation(_ context.Context, o api.Observation, v []float32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observations[o.Id] = o
	m.vectors[o.Id] = v
	return nil
}
func (m *memory) Observation(_ context.Context, id uuid.UUID) (api.Observation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.observations[id]
	if !ok {
		return o, ErrNotFound
	}
	return o, nil
}
func (m *memory) List(_ context.Context, limit int, c *Cursor) ([]api.Observation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []api.Observation{}
	for _, o := range m.observations {
		if c == nil || o.CreatedAt.Before(c.CreatedAt) || (o.CreatedAt.Equal(c.CreatedAt) && o.Id.String() < c.ID.String()) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].Id.String() > out[j].Id.String()
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *memory) Delete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.observations, id)
	delete(m.vectors, id)
	return nil
}
func (m *memory) Search(_ context.Context, v []float32, k int, exclude []uuid.UUID) ([]api.Candidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []api.Candidate{}
	for id, o := range m.observations {
		skip := false
		for _, x := range exclude {
			if x == id {
				skip = true
			}
		}
		if skip {
			continue
		}
		var score float32
		for i, n := range v {
			score += n * m.vectors[id][i]
		}
		out = append(out, api.Candidate{Observation: o, Confidence: score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence == out[j].Confidence {
			return out[i].Observation.Id.String() < out[j].Observation.Id.String()
		}
		return out[i].Confidence > out[j].Confidence
	})
	if len(out) > k {
		out = out[:k]
	}
	return out, nil
}

type memoryBlobs struct{ m *memory }

func (b memoryBlobs) Ready(context.Context) error { return nil }
func (b memoryBlobs) Put(_ context.Context, key, _ string, data []byte) error {
	b.m.mu.Lock()
	defer b.m.mu.Unlock()
	b.m.objects[key] = data
	return nil
}
func (b memoryBlobs) Get(_ context.Context, _, key string) ([]byte, error) {
	b.m.mu.Lock()
	defer b.m.mu.Unlock()
	data, ok := b.m.objects[key]
	if !ok {
		return nil, errors.New("S3 unavailable")
	}
	return data, nil
}
func (b memoryBlobs) Delete(_ context.Context, key string) error {
	b.m.mu.Lock()
	defer b.m.mu.Unlock()
	delete(b.m.objects, key)
	return nil
}

type fakeML struct {
	feature api.Feature
	err     error
}

func (f *fakeML) Ready(context.Context) error { return f.err }
func (f *fakeML) Extract(context.Context, []byte, api.BBox) (api.Feature, error) {
	return f.feature, f.err
}

func fixture(t *testing.T) (http.Handler, *memory, *fakeML) {
	t.Helper()
	m := &memory{photos: map[uuid.UUID]StoredPhoto{}, objects: map[string][]byte{}, observations: map[uuid.UUID]api.Observation{}, vectors: map[uuid.UUID][]float32{}}
	v := make([]float32, 512)
	v[0] = 1
	ml := &fakeML{feature: api.Feature{Embedding: v, ModelVersion: "test-v1"}}
	h, err := NewHTTP(&Service{Repo: m, Blobs: memoryBlobs{m}, ML: ml, Bucket: "test", ModelVersion: "test-v1"}, []string{"http://localhost:5173"})
	if err != nil {
		t.Fatal(err)
	}
	return h, m, ml
}

func call(t *testing.T, h http.Handler, method, path, kind string, body []byte, want int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if kind != "" {
		r.Header.Set("Content-Type", kind)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	// Verify actual HTTP responses against the same source contract the frontend
	// consumes, including required nulls, error bodies and binary Content-Type.
	template := r.URL.Path
	segments := strings.Split(template, "/")
	if strings.HasPrefix(template, "/api/v1/photos/") {
		segments[4] = "{photo_id}"
	}
	if strings.HasPrefix(template, "/api/v1/gallery/observations/") {
		segments[5] = "{observation_id}"
	}
	template = strings.Join(segments, "/")
	spec, err := api.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	operation := spec.Paths.Value(template).GetOperation(method)
	response := operation.Responses.Value(strconv.Itoa(w.Code))
	if response == nil {
		t.Fatalf("undocumented status %d for %s %s", w.Code, method, template)
	}
	if w.Code != 204 {
		media := response.Value.Content[w.Header().Get("Content-Type")]
		if media == nil {
			t.Fatalf("undocumented response Content-Type %s", w.Header().Get("Content-Type"))
		}
		if w.Header().Get("Content-Type") == "application/json" {
			var value any
			if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
				t.Fatal(err)
			}
			if err := media.Schema.Value.VisitJSON(value); err != nil {
				t.Fatalf("response violates OpenAPI: %v", err)
			}
		}
	}
	return w
}
func jsonCall(t *testing.T, h http.Handler, path string, body any, want int) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return call(t, h, "POST", path, "application/json", data, want)
}
func readJSON[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func multipartBytes(t *testing.T, data []byte) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, err := w.CreateFormFile("image", "test.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = w.Close()
	return b.Bytes(), w.FormDataContentType()
}
func upload(t *testing.T, h http.Handler) api.Photo {
	t.Helper()
	data, kind := multipartBytes(t, pngBytes(t))
	return readJSON[api.Photo](t, call(t, h, "POST", "/api/v1/photos", kind, data, 201))
}

func TestHTTPWorkflow(t *testing.T) {
	h, _, _ := fixture(t)
	call(t, h, "GET", "/healthz", "", nil, 200)
	call(t, h, "GET", "/readyz", "", nil, 200)
	call(t, h, "GET", "/openapi.json", "", nil, 200)
	p := upload(t, h)
	if p.Width != 8 || p.Height != 8 || p.ContentType != api.Imagepng {
		t.Fatalf("wrong photo: %+v", p)
	}
	call(t, h, "GET", "/api/v1/photos/"+p.Id.String(), "", nil, 200)
	imageResponse := call(t, h, "GET", "/api/v1/photos/"+p.Id.String()+"/content", "", nil, 200)
	if !bytes.Equal(imageResponse.Body.Bytes(), pngBytes(t)) {
		t.Fatal("download bytes changed")
	}
	body := map[string]any{"photo_id": p.Id, "bbox": map[string]int{"x": 0, "y": 0, "w": 8, "h": 8}}
	f := readJSON[api.Feature](t, jsonCall(t, h, "/api/v1/features", body, 200))
	if len(f.Embedding) != 512 {
		t.Fatal("bad embedding")
	}
	result := readJSON[api.SearchResult](t, jsonCall(t, h, "/api/v1/search", body, 200))
	if !result.Refused || result.RefusalReason == nil || *result.RefusalReason != api.EmptyGallery || result.Candidates == nil {
		t.Fatalf("bad empty result %+v", result)
	}
	o := readJSON[api.Observation](t, jsonCall(t, h, "/api/v1/gallery/observations", body, 201))
	jsonCall(t, h, "/api/v1/gallery/observations", body, 201)
	call(t, h, "GET", "/api/v1/gallery/observations/"+o.Id.String(), "", nil, 200)
	page := readJSON[api.ObservationPage](t, call(t, h, "GET", "/api/v1/gallery/observations?limit=1", "", nil, 200))
	if len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal("missing page cursor")
	}
	page2 := readJSON[api.ObservationPage](t, call(t, h, "GET", "/api/v1/gallery/observations?limit=1&cursor="+*page.NextCursor, "", nil, 200))
	if len(page2.Items) != 1 || page2.NextCursor != nil || page2.Items[0].Id == page.Items[0].Id {
		t.Fatal("bad pagination")
	}
	result = readJSON[api.SearchResult](t, jsonCall(t, h, "/api/v1/search", body, 200))
	if result.Refused || len(result.Candidates) != 2 || result.Candidates[0].Confidence != 1 {
		t.Fatalf("bad matches %+v", result)
	}
	body["exclude_observation_ids"] = []uuid.UUID{page.Items[0].Id, page2.Items[0].Id}
	result = readJSON[api.SearchResult](t, jsonCall(t, h, "/api/v1/search", body, 200))
	if !result.Refused || *result.RefusalReason != api.EmptyGallery {
		t.Fatal("exclusions failed")
	}
	body["mode"] = "ranking"
	result = readJSON[api.SearchResult](t, jsonCall(t, h, "/api/v1/search", body, 200))
	if result.Refused || result.Threshold != nil || result.RefusalReason != nil {
		t.Fatal("ranking must not refuse")
	}
	for _, id := range []uuid.UUID{page.Items[0].Id, page2.Items[0].Id} {
		call(t, h, "DELETE", "/api/v1/gallery/observations/"+id.String(), "", nil, 204)
		call(t, h, "DELETE", "/api/v1/gallery/observations/"+id.String(), "", nil, 204)
	}
	call(t, h, "GET", "/api/v1/gallery/observations/"+o.Id.String(), "", nil, 404)
	call(t, h, "GET", "/api/v1/photos/"+p.Id.String()+"/content", "", nil, 200)
}

func TestHTTPValidationAndFailures(t *testing.T) {
	h, m, ml := fixture(t)
	p := upload(t, h)
	valid := `{"photo_id":"` + p.Id.String() + `","bbox":{"x":0,"y":0,"w":8,"h":8}}`
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"invalid_json", "{", 400}, {"trailing_json", valid + "{}", 400}, {"missing", "{}", 422}, {"null", "null", 422}, {"uuid", strings.Replace(valid, p.Id.String(), "bad", 1), 400}, {"negative", strings.Replace(valid, `"x":0`, `"x":-1`, 1), 422}, {"outside", strings.Replace(valid, `"w":8`, `"w":9`, 1), 422}, {"unknown", strings.Replace(valid, `"bbox":`, `"extra":true,"bbox":`, 1), 422}, {"null_bbox", `{"photo_id":"` + p.Id.String() + `","bbox":null}`, 422}, {"invalid_mode", strings.TrimSuffix(valid, "}") + `,"mode":"bad"}`, 422}, {"top_k", strings.TrimSuffix(valid, "}") + `,"top_k":101}`, 422}, {"ranking_threshold", strings.TrimSuffix(valid, "}") + `,"mode":"ranking","threshold":0.5}`, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := call(t, h, "POST", "/api/v1/search", "application/json", []byte(tc.body), tc.status)
			e := readJSON[api.Error](t, w)
			if e.Code == "" || e.Message == "" {
				t.Fatal("missing structured error")
			}
		})
	}
	call(t, h, "POST", "/api/v1/search", "text/plain", []byte(valid), 415)
	call(t, h, "POST", "/api/v1/search", "application/json", bytes.Repeat([]byte(" "), (64<<10)+1), 413)
	call(t, h, "GET", "/api/v1/photos/not-uuid", "", nil, 400)
	call(t, h, "GET", "/api/v1/photos/"+uuid.NewString(), "", nil, 404)
	for _, q := range []string{"limit=0", "limit=101", "limit=no", "cursor=bad"} {
		call(t, h, "GET", "/api/v1/gallery/observations?"+q, "", nil, 400)
	}
	body, kind := multipartBytes(t, []byte("not image"))
	call(t, h, "POST", "/api/v1/photos", kind, body, 415)
	body, kind = multipartBytes(t, pngBytes(t)[:40])
	call(t, h, "POST", "/api/v1/photos", kind, body, 422)
	body, kind = multipartBytes(t, bytes.Repeat([]byte{1}, MaxImageBytes+1))
	call(t, h, "POST", "/api/v1/photos", kind, body, 413)
	call(t, h, "POST", "/api/v1/photos", "multipart/form-data; boundary=x", bytes.Repeat([]byte{1}, (11<<20)+1), 413)
	ml.err = errors.New("ML timeout")
	call(t, h, "GET", "/readyz", "", nil, 503)
	call(t, h, "POST", "/api/v1/search", "application/json", []byte(valid), 503)
	ml.err = nil
	ml.feature.Embedding = make([]float32, 512)
	call(t, h, "POST", "/api/v1/features", "application/json", []byte(valid), 503)
	before := len(m.objects)
	m.failSave = true
	body, kind = multipartBytes(t, pngBytes(t))
	call(t, h, "POST", "/api/v1/photos", kind, body, 503)
	if len(m.objects) != before {
		t.Fatal("orphan object not cleaned up")
	}
}

func TestThresholdAndInvalidVectors(t *testing.T) {
	h, _, ml := fixture(t)
	p := upload(t, h)
	body := map[string]any{"photo_id": p.Id, "bbox": map[string]int{"x": 0, "y": 0, "w": 8, "h": 8}}
	jsonCall(t, h, "/api/v1/gallery/observations", body, 201)
	v := make([]float32, 512)
	v[1] = 1
	ml.feature.Embedding = v
	result := readJSON[api.SearchResult](t, jsonCall(t, h, "/api/v1/search", body, 200))
	if !result.Refused || *result.RefusalReason != api.BelowThreshold {
		t.Fatal("below threshold refusal failed")
	}
	body["threshold"] = 0
	result = readJSON[api.SearchResult](t, jsonCall(t, h, "/api/v1/search", body, 200))
	if result.Refused || len(result.Candidates) != 1 {
		t.Fatal("inclusive threshold failed")
	}
	delete(body, "threshold")
	body["mode"] = "ranking"
	result = readJSON[api.SearchResult](t, jsonCall(t, h, "/api/v1/search", body, 200))
	if len(result.Candidates) != 1 || result.Candidates[0].Confidence != 0 {
		t.Fatal("ranking discarded low similarity")
	}
	for _, f := range []api.Feature{{Embedding: make([]float32, 511), ModelVersion: "test-v1"}, {Embedding: []float32{float32(math.NaN())}, ModelVersion: "test-v1"}, {Embedding: v, ModelVersion: "wrong"}} {
		if ValidateFeature(f, "test-v1") == nil {
			t.Fatal("invalid feature accepted")
		}
	}
}

func TestCORS(t *testing.T) {
	h, _, _ := fixture(t)
	for _, origin := range []string{"http://localhost:5173", "https://untrusted.example"} {
		r := httptest.NewRequest("OPTIONS", "/api/v1/search", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", "POST")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if origin == "http://localhost:5173" {
			if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != origin {
				t.Fatal("preflight failed")
			}
		} else if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted origin allowed")
		}
	}
}

func TestRealHTTPClient(t *testing.T) {
	h, _, _ := fixture(t)
	server := httptest.NewServer(h)
	defer server.Close()
	client, err := api.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.GetHealthWithResponse(context.Background())
	if err != nil || res.JSON200 == nil {
		t.Fatalf("generated client failed: %v", err)
	}
	response, err := http.Get(server.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if !json.Valid(data) {
		t.Fatal("invalid published contract")
	}
}
