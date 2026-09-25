package storage

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/repyg/brightest_teeth/backend/api"
	"github.com/repyg/brightest_teeth/backend/internal/service"
)

func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run against PostgreSQL/pgvector")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schemaName := "reid_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schemaName+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schemaName+",public")
	u.RawQuery = query.Encode()
	p, err := OpenPostgres(ctx, u.String(), "test-v1")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Pool.Close()
	photo := service.StoredPhoto{Photo: api.Photo{Id: uuid.New(), ContentType: api.Imagepng, Width: 8, Height: 8, SizeBytes: 100, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}, Bucket: "test", Key: "test.png"}
	if err := p.SavePhoto(ctx, photo); err != nil {
		t.Fatal(err)
	}
	stored, err := p.Photo(ctx, photo.Id)
	if err != nil || stored.Width != 8 {
		t.Fatalf("photo round trip: %+v %v", stored, err)
	}
	v := make([]float32, 512)
	v[0] = 1
	first := api.Observation{Id: uuid.MustParse("00000000-0000-4000-8000-000000000001"), PhotoId: photo.Id, Bbox: api.BBox{W: 8, H: 8}, ModelVersion: "test-v1", CreatedAt: photo.CreatedAt}
	second := first
	second.Id = uuid.MustParse("00000000-0000-4000-8000-000000000002")
	for _, o := range []api.Observation{second, first} {
		if err := p.SaveObservation(ctx, o, v); err != nil {
			t.Fatal(err)
		}
	}
	matches, err := p.Search(ctx, v, 1, nil)
	if err != nil || len(matches) != 1 || matches[0].Observation.Id != first.Id || matches[0].Confidence != 1 {
		t.Fatalf("search/tie order: %+v %v", matches, err)
	}
	matches, err = p.Search(ctx, v, 10, []uuid.UUID{first.Id})
	if err != nil || len(matches) != 1 || matches[0].Observation.Id != second.Id {
		t.Fatalf("exclusions/fallback: %+v %v", matches, err)
	}
	matches, err = p.Search(ctx, v, 10, []uuid.UUID{first.Id, second.Id})
	if err != nil || len(matches) != 0 {
		t.Fatalf("empty search: %+v %v", matches, err)
	}
	items, err := p.List(ctx, 1, nil)
	if err != nil || len(items) != 1 || items[0].Id != second.Id {
		t.Fatalf("list: %+v %v", items, err)
	}
	items, err = p.List(ctx, 1, &service.Cursor{CreatedAt: second.CreatedAt, ID: second.Id})
	if err != nil || len(items) != 1 || items[0].Id != first.Id {
		t.Fatalf("cursor: %+v %v", items, err)
	}
	if incompatible, err := OpenPostgres(ctx, u.String(), "test-v2"); err == nil {
		incompatible.Pool.Close()
		t.Fatal("mixed model versions accepted")
	}
	for _, id := range []uuid.UUID{first.Id, second.Id} {
		if err := p.Delete(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.Delete(ctx, first.Id); err != nil {
		t.Fatal(err)
	}
	if err := p.migrate(ctx); err != nil {
		t.Fatalf("migration not idempotent: %v", err)
	}
}

func TestS3Integration(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set TEST_S3_ENDPOINT to run against MinIO")
	}
	bucket := "reid-test-" + uuid.NewString()
	key := "test.png"
	access, secret := os.Getenv("TEST_S3_ACCESS_KEY_ID"), os.Getenv("TEST_S3_SECRET_ACCESS_KEY")
	if access == "" {
		access = "minioadmin"
	}
	if secret == "" {
		secret = "minioadmin"
	}
	s, err := NewS3(endpoint, "us-east-1", bucket, access, secret)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := s.Client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Delete(cleanup, key)
		if err := s.Client.RemoveBucket(cleanup, bucket); err != nil {
			t.Error(err)
		}
	}()
	if err := s.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, key, "image/png", []byte("stored bytes")); err != nil {
		t.Fatal(err)
	}
	data, err := s.Get(ctx, bucket, key)
	if err != nil || string(data) != "stored bytes" {
		t.Fatalf("S3 roundtrip: %q %v", data, err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
}
