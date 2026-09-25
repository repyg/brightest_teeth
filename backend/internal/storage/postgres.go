package storage

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/repyg/brightest_teeth/backend/api"
	"github.com/repyg/brightest_teeth/backend/internal/service"
)

//go:embed migrations/001_schema.sql
var schema string

type Postgres struct {
	Pool    *pgxpool.Pool
	Version string
}

func OpenPostgres(ctx context.Context, dsn, version string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	p := &Postgres{pool, version}
	if err := p.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return p, nil
}

func (p *Postgres) migrate(ctx context.Context) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(74190512)"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, schema); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO reid_model_state (model_version) VALUES ($1) ON CONFLICT DO NOTHING", p.Version); err != nil {
		return err
	}
	var current string
	if err := tx.QueryRow(ctx, "SELECT model_version FROM reid_model_state WHERE singleton FOR UPDATE").Scan(&current); err != nil {
		return err
	}
	var incompatible bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vehicle_observations o LEFT JOIN vehicle_photos p ON p.id=o.photo_id
        WHERE o.metadata->>'model_version' IS DISTINCT FROM $1 OR p.id IS NULL OR p.width IS NULL OR p.height IS NULL)`, p.Version).Scan(&incompatible); err != nil {
		return err
	}
	if incompatible {
		return fmt.Errorf("gallery contains legacy or incompatible observations: backfill photo dimensions and reindex with model %s before startup", p.Version)
	}
	if current != p.Version {
		if _, err := tx.Exec(ctx, "UPDATE reid_model_state SET model_version=$1", p.Version); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) Ready(ctx context.Context) error {
	var current string
	if err := p.Pool.QueryRow(ctx, "SELECT model_version FROM reid_model_state WHERE singleton").Scan(&current); err != nil {
		return err
	}
	if current != p.Version {
		return fmt.Errorf("gallery model version changed")
	}
	return nil
}

func (p *Postgres) SavePhoto(ctx context.Context, v service.StoredPhoto) error {
	_, err := p.Pool.Exec(ctx, `INSERT INTO vehicle_photos (id,bucket,object_key,content_type,size_bytes,width,height,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, v.Id, v.Bucket, v.Key, string(v.ContentType), v.SizeBytes, v.Width, v.Height, v.CreatedAt)
	return err
}

func (p *Postgres) Photo(ctx context.Context, id uuid.UUID) (service.StoredPhoto, error) {
	var v service.StoredPhoto
	var kind string
	err := p.Pool.QueryRow(ctx, `SELECT id,bucket,object_key,content_type,size_bytes,width,height,created_at FROM vehicle_photos WHERE id=$1`, id).Scan(&v.Id, &v.Bucket, &v.Key, &kind, &v.SizeBytes, &v.Width, &v.Height, &v.CreatedAt)
	v.ContentType = api.PhotoContentType(kind)
	if errors.Is(err, pgx.ErrNoRows) {
		err = service.ErrNotFound
	}
	return v, err
}

func vector(v []float32) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.FormatFloat(float64(n), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

type metadata struct {
	ImageID      *string `json:"image_id,omitempty"`
	VehicleID    *string `json:"vehicle_id,omitempty"`
	ModelVersion string  `json:"model_version"`
}

func (p *Postgres) SaveObservation(ctx context.Context, o api.Observation, embedding []float32) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var version string
	if err := tx.QueryRow(ctx, "SELECT model_version FROM reid_model_state WHERE singleton FOR SHARE").Scan(&version); err != nil {
		return err
	}
	if version != o.ModelVersion {
		return fmt.Errorf("model version mismatch")
	}
	data, err := json.Marshal(metadata{o.ImageId, o.VehicleId, o.ModelVersion})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO vehicle_observations (id,photo_id,bbox,embedding,metadata,created_at) VALUES ($1,$2,$3,$4::vector,$5,$6)`, o.Id, o.PhotoId, []int32{o.Bbox.X, o.Bbox.Y, o.Bbox.W, o.Bbox.H}, vector(embedding), data, o.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const observationColumns = "id,photo_id,bbox,metadata,created_at"

func scanObservation(row pgx.Row, score *float64) (api.Observation, error) {
	var o api.Observation
	var bbox []int32
	var data []byte
	args := []any{&o.Id, &o.PhotoId, &bbox, &data, &o.CreatedAt}
	if score != nil {
		args = append(args, score)
	}
	if err := row.Scan(args...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = service.ErrNotFound
		}
		return o, err
	}
	if len(bbox) != 4 {
		return o, fmt.Errorf("invalid stored bbox")
	}
	o.Bbox = api.BBox{X: bbox[0], Y: bbox[1], W: bbox[2], H: bbox[3]}
	var meta metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return o, err
	}
	o.ImageId, o.VehicleId, o.ModelVersion = meta.ImageID, meta.VehicleID, meta.ModelVersion
	return o, nil
}

func (p *Postgres) Observation(ctx context.Context, id uuid.UUID) (api.Observation, error) {
	return scanObservation(p.Pool.QueryRow(ctx, "SELECT "+observationColumns+" FROM vehicle_observations WHERE id=$1", id), nil)
}

func (p *Postgres) List(ctx context.Context, limit int, c *service.Cursor) ([]api.Observation, error) {
	query := "SELECT " + observationColumns + " FROM vehicle_observations"
	args := []any{limit}
	if c != nil {
		query += " WHERE (created_at,id) < ($2,$3)"
		args = append(args, c.CreatedAt, c.ID)
	}
	query += " ORDER BY created_at DESC,id DESC LIMIT $1"
	rows, err := p.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []api.Observation{}
	for rows.Next() {
		o, err := scanObservation(rows, nil)
		if err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

func (p *Postgres) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := p.Pool.Exec(ctx, "DELETE FROM vehicle_observations WHERE id=$1", id)
	return err
}

func (p *Postgres) Search(ctx context.Context, embedding []float32, limit int, exclude []uuid.UUID) ([]api.Candidate, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var version string
	if err := tx.QueryRow(ctx, "SELECT model_version FROM reid_model_state WHERE singleton FOR SHARE").Scan(&version); err != nil {
		return nil, err
	}
	if version != p.Version {
		return nil, fmt.Errorf("gallery model version changed")
	}
	if _, err := tx.Exec(ctx, "SET LOCAL hnsw.iterative_scan = strict_order; SET LOCAL hnsw.ef_search = 200"); err != nil {
		return nil, err
	}
	ids := make([]string, len(exclude))
	for i, id := range exclude {
		ids[i] = id.String()
	}
	// Materialize the ANN nearest neighbours, then give equal scores a stable order.
	rows, err := tx.Query(ctx, `WITH nearest AS MATERIALIZED (
        SELECT `+observationColumns+`, embedding <=> $1::vector AS distance
        FROM vehicle_observations WHERE NOT (id = ANY($2::uuid[]))
        ORDER BY embedding <=> $1::vector FETCH FIRST $3 ROWS WITH TIES
    ) SELECT `+observationColumns+`, 1-distance FROM nearest ORDER BY distance,id LIMIT $3`, vector(embedding), ids, limit)
	if err != nil {
		return nil, err
	}
	result := []api.Candidate{}
	for rows.Next() {
		var score float64
		o, err := scanObservation(rows, &score)
		if err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, api.Candidate{Observation: o, Confidence: float32(math.Max(-1, math.Min(1, score)))})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// ANN filtering can exhaust its scan budget. Exact fallback distinguishes an
	// actually empty gallery from index underfill, especially after exclusions.
	if len(result) < limit {
		if _, err := tx.Exec(ctx, "SET LOCAL enable_indexscan = off; SET LOCAL enable_bitmapscan = off"); err != nil {
			return nil, err
		}
		rows, err = tx.Query(ctx, `SELECT `+observationColumns+`, 1-(embedding <=> $1::vector) FROM vehicle_observations WHERE NOT (id = ANY($2::uuid[])) ORDER BY embedding <=> $1::vector,id LIMIT $3`, vector(embedding), ids, limit)
		if err != nil {
			return nil, err
		}
		result = []api.Candidate{}
		for rows.Next() {
			var score float64
			o, err := scanObservation(rows, &score)
			if err != nil {
				rows.Close()
				return nil, err
			}
			result = append(result, api.Candidate{Observation: o, Confidence: float32(math.Max(-1, math.Min(1, score)))})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
