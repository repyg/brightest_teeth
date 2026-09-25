import type {
  ApiErrorPayload,
  BBox,
  Feature,
  Observation,
  ObservationPage,
  Photo,
  SearchMode,
  SearchResult,
} from "./types";

export const API_URL = (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080").replace(/\/$/, "");

export class ApiError extends Error {
  status: number;
  code: string;
  details?: ApiErrorPayload["details"];

  constructor(status: number, payload: ApiErrorPayload) {
    super(payload.message || "Не удалось выполнить запрос");
    this.name = "ApiError";
    this.status = status;
    this.code = payload.code || "unknown_error";
    this.details = payload.details;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_URL}${path}`, {
      ...init,
      headers: init?.body instanceof FormData
        ? init.headers
        : { "Content-Type": "application/json", ...init?.headers },
    });
  } catch {
    throw new ApiError(0, {
      code: "network_error",
      message: "Не удалось подключиться к API. Проверьте, что backend запущен.",
    });
  }

  if (response.status === 204) return undefined as T;

  const body = await response.json().catch(() => null);
  if (!response.ok) {
    throw new ApiError(response.status, body || {
      code: "invalid_response",
      message: "Сервис вернул некорректный ответ",
    });
  }
  return body as T;
}

export const api = {
  readiness: (signal?: AbortSignal) => request<{ status: "ok" }>("/readyz", { signal }),

  uploadPhoto(file: File) {
    const form = new FormData();
    form.append("image", file);
    return request<Photo>("/api/v1/photos", { method: "POST", body: form });
  },

  extractFeature(input: { photo_id: string; bbox: BBox }) {
    return request<Feature>("/api/v1/features", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },

  addObservation(input: {
    photo_id: string;
    bbox: BBox;
    image_id?: string;
    vehicle_id?: string;
  }) {
    return request<Observation>("/api/v1/gallery/observations", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },

  listObservations(cursor?: string, limit = 20) {
    const params = new URLSearchParams({ limit: String(limit) });
    if (cursor) params.set("cursor", cursor);
    return request<ObservationPage>(`/api/v1/gallery/observations?${params}`);
  },

  deleteObservation(id: string) {
    return request<void>(`/api/v1/gallery/observations/${id}`, { method: "DELETE" });
  },

  search(input: {
    photo_id: string;
    bbox: BBox;
    mode: SearchMode;
    top_k: number;
    threshold?: number;
    exclude_observation_ids?: string[];
  }) {
    return request<SearchResult>("/api/v1/search", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
};

export const photoContentUrl = (photoId: string) =>
  `${API_URL}/api/v1/photos/${encodeURIComponent(photoId)}/content`;
