package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/repyg/brightest_teeth/backend/api"
)

const MaxImageBytes = 10 << 20

type Service struct {
	Repo         Repository
	Blobs        BlobStore
	ML           Extractor
	Bucket       string
	ModelVersion string
}

var _ api.StrictServerInterface = (*Service)(nil)

func (s *Service) GetOpenAPIDocument(context.Context, api.GetOpenAPIDocumentRequestObject) (api.GetOpenAPIDocumentResponseObject, error) {
	spec, err := api.GetSwagger()
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	var document map[string]interface{}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	return api.GetOpenAPIDocument200JSONResponse(document), nil
}

func (s *Service) GetHealth(context.Context, api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	return api.GetHealth200JSONResponse{Status: api.Ok}, nil
}

func (s *Service) GetReadiness(ctx context.Context, _ api.GetReadinessRequestObject) (api.GetReadinessResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, check := range []func(context.Context) error{s.Repo.Ready, s.Blobs.Ready, s.ML.Ready} {
		if err := check(ctx); err != nil {
			return nil, dependency(err)
		}
	}
	return api.GetReadiness200JSONResponse{Status: api.Ok}, nil
}

func (s *Service) UploadPhoto(ctx context.Context, req api.UploadPhotoRequestObject) (api.UploadPhotoResponseObject, error) {
	if req.Body == nil {
		return nil, fail(400, api.ErrorCodeBadRequest, "Отсутствует multipart body")
	}
	var data []byte
	for {
		part, err := req.Body.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, multipartError(err)
		}
		if part.FormName() != "image" || part.FileName() == "" || data != nil {
			_ = part.Close()
			return nil, invalid("Ожидается ровно один файл в поле image")
		}
		data, err = io.ReadAll(io.LimitReader(part, MaxImageBytes+1))
		if err != nil {
			return nil, multipartError(err)
		}
		if len(data) > MaxImageBytes {
			return nil, fail(413, api.ErrorCodePayloadTooLarge, "Файл превышает 10 MiB")
		}
		if err := part.Close(); err != nil {
			return nil, multipartError(err)
		}
	}
	if len(data) == 0 {
		return nil, invalid("Файл image обязателен и не должен быть пустым")
	}
	kind := http.DetectContentType(data)
	if kind != "image/jpeg" && kind != "image/png" {
		return nil, fail(415, api.ErrorCodeUnsupportedMediaType, "Поддерживаются JPEG и PNG")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, invalid("Не удалось прочитать изображение")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return nil, invalid("Изображение превышает 40 млн пикселей")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return nil, invalid("Изображение повреждено")
	}
	p := StoredPhoto{Photo: api.Photo{Id: uuid.New(), Width: cfg.Width, Height: cfg.Height, SizeBytes: int64(len(data)), ContentType: api.PhotoContentType(kind), CreatedAt: time.Now().UTC()}, Bucket: s.Bucket}
	p.Key = "photos/" + p.Id.String()
	if err := s.Blobs.Put(ctx, p.Key, kind, data); err != nil {
		return nil, dependency(err)
	}
	if err := s.Repo.SavePhoto(ctx, p); err != nil {
		// Cleanup must still run if the original request was cancelled.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if cleanupErr := s.Blobs.Delete(cleanup, p.Key); cleanupErr != nil {
			slog.Error("orphaned photo", "key", p.Key, "error", cleanupErr)
		}
		return nil, dependency(err)
	}
	return api.UploadPhoto201JSONResponse(p.Photo), nil
}

func multipartError(err error) error {
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		return fail(413, api.ErrorCodePayloadTooLarge, "Превышен размер multipart")
	}
	return fail(400, api.ErrorCodeBadRequest, "Некорректный multipart")
}

func (s *Service) GetPhoto(ctx context.Context, req api.GetPhotoRequestObject) (api.GetPhotoResponseObject, error) {
	p, err := s.Repo.Photo(ctx, req.PhotoId)
	if err != nil {
		return nil, dependency(err)
	}
	return api.GetPhoto200JSONResponse(p.Photo), nil
}

func (s *Service) DownloadPhoto(ctx context.Context, req api.DownloadPhotoRequestObject) (api.DownloadPhotoResponseObject, error) {
	p, err := s.Repo.Photo(ctx, req.PhotoId)
	if err != nil {
		return nil, dependency(err)
	}
	data, err := s.Blobs.Get(ctx, p.Bucket, p.Key)
	if err != nil {
		return nil, dependency(err)
	}
	if p.ContentType == "image/png" {
		return api.DownloadPhoto200ImagepngResponse{Body: bytes.NewReader(data), ContentLength: int64(len(data))}, nil
	}
	return api.DownloadPhoto200ImagejpegResponse{Body: bytes.NewReader(data), ContentLength: int64(len(data))}, nil
}

func (s *Service) feature(ctx context.Context, id uuid.UUID, bbox api.BBox) (api.Feature, error) {
	p, err := s.Repo.Photo(ctx, id)
	if err != nil {
		return api.Feature{}, dependency(err)
	}
	if bbox.X < 0 || bbox.Y < 0 || bbox.W < 1 || bbox.H < 1 || int64(bbox.X)+int64(bbox.W) > int64(p.Width) || int64(bbox.Y)+int64(bbox.H) > int64(p.Height) {
		return api.Feature{}, invalid("BBox должен полностью находиться внутри кадра")
	}
	data, err := s.Blobs.Get(ctx, p.Bucket, p.Key)
	if err != nil {
		return api.Feature{}, dependency(err)
	}
	f, err := s.ML.Extract(ctx, data, bbox)
	if err != nil {
		return api.Feature{}, dependency(err)
	}
	if err := ValidateFeature(f, s.ModelVersion); err != nil {
		return api.Feature{}, dependency(err)
	}
	return f, nil
}

func ValidateFeature(f api.Feature, version string) error {
	if f.ModelVersion != version || len(f.Embedding) != 512 {
		return fmt.Errorf("incompatible model response")
	}
	var norm float64
	for _, v := range f.Embedding {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v < -1 || v > 1 {
			return fmt.Errorf("invalid embedding component")
		}
		norm += float64(v) * float64(v)
	}
	if math.Abs(math.Sqrt(norm)-1) > 1e-5 {
		return fmt.Errorf("embedding is not normalized")
	}
	return nil
}

func (s *Service) ExtractFeature(ctx context.Context, req api.ExtractFeatureRequestObject) (api.ExtractFeatureResponseObject, error) {
	f, err := s.feature(ctx, req.Body.PhotoId, req.Body.Bbox)
	if err != nil {
		return nil, err
	}
	return api.ExtractFeature200JSONResponse(f), nil
}

func (s *Service) CreateObservation(ctx context.Context, req api.CreateObservationRequestObject) (api.CreateObservationResponseObject, error) {
	f, err := s.feature(ctx, req.Body.PhotoId, req.Body.Bbox)
	if err != nil {
		return nil, err
	}
	o := api.Observation{Id: uuid.New(), PhotoId: req.Body.PhotoId, Bbox: req.Body.Bbox, ImageId: req.Body.ImageId, VehicleId: req.Body.VehicleId, ModelVersion: f.ModelVersion, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
	if err := s.Repo.SaveObservation(ctx, o, f.Embedding); err != nil {
		return nil, dependency(err)
	}
	return api.CreateObservation201JSONResponse(o), nil
}

func (s *Service) GetObservation(ctx context.Context, req api.GetObservationRequestObject) (api.GetObservationResponseObject, error) {
	o, err := s.Repo.Observation(ctx, req.ObservationId)
	if err != nil {
		return nil, dependency(err)
	}
	return api.GetObservation200JSONResponse(o), nil
}

func (s *Service) DeleteObservation(ctx context.Context, req api.DeleteObservationRequestObject) (api.DeleteObservationResponseObject, error) {
	if err := s.Repo.Delete(ctx, req.ObservationId); err != nil {
		return nil, dependency(err)
	}
	return api.DeleteObservation204Response{}, nil
}

func (s *Service) ListObservations(ctx context.Context, req api.ListObservationsRequestObject) (api.ListObservationsResponseObject, error) {
	limit := 20
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if limit < 1 || limit > 100 {
		return nil, fail(400, api.ErrorCodeBadRequest, "limit должен быть от 1 до 100")
	}
	var cursor *Cursor
	if req.Params.Cursor != nil {
		encoded := *req.Params.Cursor
		if len(encoded) == 0 || len(encoded) > 512 {
			return nil, fail(400, api.ErrorCodeBadRequest, "Некорректный cursor")
		}
		data, err := base64.RawURLEncoding.DecodeString(encoded)
		var c Cursor
		if err != nil || json.Unmarshal(data, &c) != nil || c.ID == uuid.Nil || c.CreatedAt.IsZero() {
			return nil, fail(400, api.ErrorCodeBadRequest, "Некорректный cursor")
		}
		cursor = &c
	}
	items, err := s.Repo.List(ctx, limit+1, cursor)
	if err != nil {
		return nil, dependency(err)
	}
	result := api.ObservationPage{Items: items}
	if result.Items == nil {
		result.Items = []api.Observation{}
	}
	if len(items) > limit {
		last := items[limit-1]
		data, _ := json.Marshal(Cursor{CreatedAt: last.CreatedAt, ID: last.Id})
		next := base64.RawURLEncoding.EncodeToString(data)
		result.NextCursor = &next
		result.Items = items[:limit]
	}
	return api.ListObservations200JSONResponse(result), nil
}

func (s *Service) SearchVehicles(ctx context.Context, req api.SearchVehiclesRequestObject) (api.SearchVehiclesResponseObject, error) {
	body := req.Body
	mode, topK := api.Candidates, 10
	if body.Mode != nil {
		mode = *body.Mode
	}
	if body.TopK != nil {
		topK = *body.TopK
	}
	if mode == api.Ranking && body.Threshold != nil {
		return nil, invalid("threshold нельзя передавать в режиме ranking")
	}
	f, err := s.feature(ctx, body.PhotoId, body.Bbox)
	if err != nil {
		return nil, err
	}
	exclude := []uuid.UUID{}
	if body.ExcludeObservationIds != nil {
		exclude = *body.ExcludeObservationIds
	}
	candidates, err := s.Repo.Search(ctx, f.Embedding, topK, exclude)
	if err != nil {
		return nil, dependency(err)
	}
	result := api.SearchResult{Candidates: []api.Candidate{}, Mode: mode, ModelVersion: f.ModelVersion}
	if mode == api.Ranking {
		result.Candidates = append(result.Candidates, candidates...)
	} else {
		threshold := float32(0.36)
		if body.Threshold != nil {
			threshold = *body.Threshold
		}
		result.Threshold = &threshold
		for _, c := range candidates {
			if c.Confidence >= threshold {
				result.Candidates = append(result.Candidates, c)
			}
		}
		if len(result.Candidates) == 0 {
			result.Refused = true
			reason := api.BelowThreshold
			if len(candidates) == 0 {
				reason = api.EmptyGallery
			}
			result.RefusalReason = &reason
		}
	}
	return api.SearchVehicles200JSONResponse(result), nil
}
