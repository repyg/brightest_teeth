package service

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/repyg/brightest_teeth/backend/api"
)

type HTTPError struct {
	Status  int
	Code    api.ErrorCode
	Message string
}

func (e *HTTPError) Error() string { return e.Message }

func fail(status int, code api.ErrorCode, message string) error {
	return &HTTPError{status, code, message}
}

func invalid(message string) error { return fail(422, api.ErrorCodeValidationError, message) }

func dependency(err error) error {
	if errors.Is(err, ErrNotFound) {
		return fail(404, api.ErrorCodeNotFound, "Ресурс не найден")
	}
	slog.Error("dependency failure", "error", err)
	return fail(503, api.ErrorCodeDependencyUnavailable, "Хранилище или ML-сервис недоступны")
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var problem *HTTPError
	if !errors.As(err, &problem) {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			problem = &HTTPError{413, api.ErrorCodePayloadTooLarge, "Превышен размер запроса"}
		} else {
			slog.Error("request failed", "path", r.URL.Path, "error", err)
			problem = &HTTPError{500, api.ErrorCodeInternalError, "Внутренняя ошибка сервиса"}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(api.Error{Code: problem.Code, Message: problem.Message})
}
