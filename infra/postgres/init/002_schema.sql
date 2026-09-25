CREATE TABLE IF NOT EXISTS vehicle_photos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bucket text NOT NULL,
    object_key text NOT NULL,
    content_type text,
    size_bytes bigint CHECK (size_bytes IS NULL OR size_bytes >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bucket, object_key)
);

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
    ON vehicle_observations
    USING hnsw (embedding vector_cosine_ops);

CREATE INDEX IF NOT EXISTS vehicle_observations_created_at_idx
    ON vehicle_observations (created_at DESC);

CREATE INDEX IF NOT EXISTS vehicle_observations_metadata_gin_idx
    ON vehicle_observations
    USING gin (metadata);
