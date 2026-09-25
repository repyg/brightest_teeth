# Подключение фронтенда

API: `http://localhost:8080`. Контракт: `GET /openapi.json`.
После `docker compose up -d --build` дождитесь ответа 200 от `/readyz`.
ML работает на CPU и доступен только внутри сети контейнеров.

CORS разрешает `http://localhost:5173`, `http://localhost:3000` и те же адреса
с `127.0.0.1`. Для другого origin задайте `CORS_ALLOWED_ORIGINS` (список через
запятую) и пересоздайте backend: `docker compose up -d backend`.
Это локальный MVP без авторизации. Cookie/credentials не нужны.

## Пример JavaScript

```js
const API = "http://localhost:8080";

async function api(path, options = {}) {
  const response = await fetch(API + path, options);
  if (response.status === 204) return;
  const body = await response.json();
  if (!response.ok) {
    throw Object.assign(new Error(body.message), {
      status: response.status, code: body.code, details: body.details,
    });
  }
  return body;
}
const post = (path, body) => api(path, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

// file — File из <input type="file" accept="image/jpeg,image/png">.
async function uploadPhoto(file) {
  const form = new FormData();
  form.append("image", file);
  // Content-Type с boundary устанавливает браузер.
  return api("/api/v1/photos", { method: "POST", body: form });
}

// bbox — {x, y, w, h} в пикселях исходного кадра, целые числа.
async function addToGallery(file, bbox) {
  const photo = await uploadPhoto(file);
  return post("/api/v1/gallery/observations", {
    photo_id: photo.id, bbox, image_id: file.name,
  });
}
async function search(file, bbox) {
  const photo = await uploadPhoto(file);
  return post("/api/v1/search", {
    photo_id: photo.id, bbox, mode: "candidates", top_k: 10,
  });
}
// result.refused === true: показать «Уверенных совпадений нет».
// result.candidates[].observation содержит bbox и photo_id.
const imageUrl = photoId => `${API}/api/v1/photos/${photoId}/content`;
const firstPage = () => api("/api/v1/gallery/observations?limit=20");
const nextPage = cursor => api(
  `/api/v1/gallery/observations?limit=20&cursor=${encodeURIComponent(cursor)}`
);
const removeObservation = id => api(`/api/v1/gallery/observations/${id}`, {
  method: "DELETE",
});
```

Один `photo_id` можно переиспользовать для нескольких BBox и запросов поиска.
Не загружайте кадр заново при каждой корректировке прямоугольника.
При рисовании на уменьшенном изображении пересчитывайте координаты в исходный
размер (`width/height` ответа загрузки); EXIF-ориентация сервером не применяется.

## Ручки

| Метод | Путь | Назначение |
| --- | --- | --- |
| POST | `/api/v1/photos` | Загрузка JPEG/PNG через multipart image |
| GET | `/api/v1/photos/{photo_id}` | Метаданные кадра |
| GET | `/api/v1/photos/{photo_id}/content` | Байты JPEG/PNG для `<img>` |
| POST | `/api/v1/features` | `{photo_id,bbox}` → embedding из 512 чисел |
| POST | `/api/v1/gallery/observations` | `{photo_id,bbox,image_id?,vehicle_id?}` → наблюдение |
| GET | `/api/v1/gallery/observations` | `{items,next_cursor}`, параметры limit/cursor |
| GET | `/api/v1/gallery/observations/{id}` | Наблюдение |
| DELETE | `/api/v1/gallery/observations/{id}` | 204, в том числе при повторном удалении |
| POST | `/api/v1/search` | `{photo_id,bbox,mode?,top_k?,threshold?,exclude_observation_ids?}` |
| GET | `/healthz` | Жив ли процесс |
| GET | `/readyz` | Готовы ли БД, MinIO, модель |
| GET | `/openapi.json` | Актуальный контракт |

`confidence` лежит в [-1,1] и не является вероятностью. `candidates` отсекает
значения ниже 0.36 (можно передать `threshold`). `ranking` возвращает top-K без
отсечения; передавать threshold в этом режиме нельзя. `exclude_observation_ids`
исключает наблюдения до поиска. Запрос не добавляется в галерею.

Пример отказа (HTTP 200):

```json
{
  "mode": "candidates",
  "threshold": 0.36,
  "candidates": [],
  "refused": true,
  "refusal_reason": "empty_gallery",
  "model_version": "reid-resnet50-v1"
}
```

`refusal_reason` равен `below_threshold`, если объекты есть, но порог не пройден;
при успешном совпадении и в режиме ranking — `null`. В ranking `threshold` — `null`.

Ошибки: `{"code":"validation_error","message":"..."}`.

| HTTP | code | Реакция интерфейса |
| --- | --- | --- |
| 400 | bad_request | Проверить JSON, UUID, параметры и cursor |
| 404 | not_found | Ресурс отсутствует |
| 413 | payload_too_large | Уменьшить файл/запрос |
| 415 | unsupported_media_type | Использовать JPEG/PNG или правильный Content-Type |
| 422 | validation_error | Исправить BBox или поля запроса |
| 503 | dependency_unavailable | БД/модель/MinIO временно недоступны или ML занят |
| 500 | internal_error | Показать общую ошибку |

ML выполняет один инференс одновременно; блокируйте повторный поиск до завершения
текущего. Повторять автоматически можно поиск и извлечение признаков; POST загрузки
и добавления наблюдения создаёт новый ресурс при каждом успехе.
Удаление наблюдения сохраняет исходный кадр.
