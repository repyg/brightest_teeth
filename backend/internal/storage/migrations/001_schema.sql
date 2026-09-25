CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE IF NOT EXISTS vehicle_photos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bucket text NOT NULL,
    object_key text NOT NULL,
    content_type text,
    size_bytes bigint CHECK (size_bytes IS NULL OR size_bytes >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bucket, object_key)
);
ALTER TABLE vehicle_photos ADD COLUMN IF NOT EXISTS width integer CHECK (width > 0);
ALTER TABLE vehicle_photos ADD COLUMN IF NOT EXISTS height integer CHECK (height > 0);
CREATE TABLE IF NOT EXISTS vehicle_observations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    photo_id uuid REFERENCES vehicle_photos(id) ON DELETE SET NULL,
    bbox integer[] NOT NULL,
    embedding vector(512) NOT NULL,
    confidence real CHECK (confidence IS NULL OR confidence >= 0),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT vehicle_observations_bbox_len CHECK (array_length(bbox, 1) = 4)
);
CREATE INDEX IF NOT EXISTS vehicle_observations_embedding_hnsw_idx
    ON vehicle_observations USING hnsw (embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS vehicle_observations_page_idx
    ON vehicle_observations (created_at DESC, id DESC);
CREATE TABLE IF NOT EXISTS reid_model_state (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    model_version text NOT NULL
);
