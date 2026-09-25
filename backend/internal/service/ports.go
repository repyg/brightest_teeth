package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/repyg/brightest_teeth/backend/api"
)

var ErrNotFound = errors.New("resource not found")

type StoredPhoto struct {
	api.Photo
	Bucket string
	Key    string
}

type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

type Repository interface {
	Ready(context.Context) error
	SavePhoto(context.Context, StoredPhoto) error
	Photo(context.Context, uuid.UUID) (StoredPhoto, error)
	SaveObservation(context.Context, api.Observation, []float32) error
	Observation(context.Context, uuid.UUID) (api.Observation, error)
	List(context.Context, int, *Cursor) ([]api.Observation, error)
	Delete(context.Context, uuid.UUID) error
	Search(context.Context, []float32, int, []uuid.UUID) ([]api.Candidate, error)
}

type BlobStore interface {
	Ready(context.Context) error
	Put(context.Context, string, string, []byte) error
	Get(context.Context, string, string) ([]byte, error)
	Delete(context.Context, string) error
}

type Extractor interface {
	Ready(context.Context) error
	Extract(context.Context, []byte, api.BBox) (api.Feature, error)
}
