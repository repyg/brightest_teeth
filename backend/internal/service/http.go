package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/repyg/brightest_teeth/backend/api"
)

// NewHTTP connects the generated router to the business layer. Body validation
// precedes generated decoding so missing required fields never become Go zeros.
func NewHTTP(s *Service, origins []string) (http.Handler, error) {
	spec, err := api.GetSwagger()
	if err != nil {
		return nil, err
	}
	if err := spec.Validate(context.Background()); err != nil {
		return nil, err
	}
	badRequest := func(w http.ResponseWriter, r *http.Request, err error) {
		WriteError(w, r, fail(400, api.ErrorCodeBadRequest, "Некорректный запрос"))
	}
	strict := api.NewStrictHandlerWithOptions(s, nil, api.StrictHTTPServerOptions{RequestErrorHandlerFunc: badRequest, ResponseErrorHandlerFunc: WriteError})
	mux := http.NewServeMux()
	api.HandlerWithOptions(strict, api.StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: badRequest})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, fail(404, api.ErrorCodeNotFound, "Маршрут не найден"))
	})
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		if origin = strings.TrimSpace(origin); origin != "" {
			allowed[origin] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				slog.Error("handler panic", "panic", value)
				WriteError(w, r, fmt.Errorf("handler panic"))
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			if origin != "" && !allowed[origin] {
				WriteError(w, r, fail(400, api.ErrorCodeBadRequest, "Origin не разрешён"))
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		if r.Method == http.MethodPost {
			if err := validateBody(w, r, spec); err != nil {
				WriteError(w, r, err)
				return
			}
		}
		mux.ServeHTTP(w, r)
	}), nil
}

func validateBody(w http.ResponseWriter, r *http.Request, spec *openapi3.T) error {
	path := spec.Paths.Value(r.URL.Path)
	if path == nil || path.Post == nil {
		return nil
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return fail(415, api.ErrorCodeUnsupportedMediaType, "Некорректный Content-Type")
	}
	if r.URL.Path == "/api/v1/photos" {
		if media != "multipart/form-data" {
			return fail(415, api.ErrorCodeUnsupportedMediaType, "Ожидается multipart/form-data")
		}
		// Read bounded body first, including epilogue, before storing anything.
		r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		r.Body = io.NopCloser(bytes.NewReader(data))
		return nil
	}
	if media != "application/json" {
		return fail(415, api.ErrorCodeUnsupportedMediaType, "Ожидается application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fail(400, api.ErrorCodeBadRequest, "Некорректный JSON")
	}
	// UUID format errors are 400 by the public contract, schema errors are 422.
	if obj, ok := value.(map[string]any); ok {
		if raw, ok := obj["photo_id"].(string); ok {
			if _, err := uuid.Parse(raw); err != nil {
				return fail(400, api.ErrorCodeBadRequest, "Некорректный photo_id")
			}
		}
		if ids, ok := obj["exclude_observation_ids"].([]any); ok {
			for _, id := range ids {
				if raw, ok := id.(string); ok {
					if _, err := uuid.Parse(raw); err != nil {
						return fail(400, api.ErrorCodeBadRequest, "Некорректный UUID в exclude_observation_ids")
					}
				}
			}
		}
	}
	schema := path.Post.RequestBody.Value.Content["application/json"].Schema.Value
	if err := schema.VisitJSON(value); err != nil {
		message := "Тело запроса не соответствует OpenAPI"
		if detail, ok := err.(*openapi3.SchemaError); ok {
			message += ": " + strings.Join(detail.JSONPointer(), ".") + " " + detail.Reason
		}
		return invalid(message)
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return nil
}
