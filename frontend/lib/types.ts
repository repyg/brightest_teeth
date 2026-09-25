export type BBox = { x: number; y: number; w: number; h: number };

export type Photo = {
  id: string;
  content_type: "image/jpeg" | "image/png";
  size_bytes: number;
  width: number;
  height: number;
  created_at: string;
};

export type Observation = {
  id: string;
  photo_id: string;
  bbox: BBox;
  image_id?: string;
  vehicle_id?: string;
  model_version: string;
  created_at: string;
};

export type ObservationPage = {
  items: Observation[];
  next_cursor: string | null;
};

export type Feature = {
  embedding: number[];
  model_version: string;
};

export type SearchMode = "candidates" | "ranking";

export type SearchResult = {
  mode: SearchMode;
  threshold: number | null;
  candidates: Array<{ observation: Observation; confidence: number }>;
  refused: boolean;
  refusal_reason: "empty_gallery" | "below_threshold" | null;
  model_version: string;
};

export type ApiErrorPayload = {
  code: string;
  message: string;
  details?: Array<{ field: string; message: string }>;
};
