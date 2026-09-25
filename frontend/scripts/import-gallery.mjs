import { readFile, readdir } from "node:fs/promises";
import path from "node:path";

const apiUrl = (process.env.API_URL || "http://127.0.0.1:8080").replace(/\/$/, "");
const datasetDir = path.resolve(process.env.DATASET_DIR || path.join(process.cwd(), "..", "dataset"));
// The ML service deliberately serializes inference, so one worker is the safe default.
const concurrency = Math.max(1, Number.parseInt(process.env.IMPORT_CONCURRENCY || "1", 10));
const importLimit = Number.parseInt(process.env.IMPORT_LIMIT || "0", 10);
const sources = ["train.csv", "test_gallery.csv", "test_query.csv"];

function parseCsv(text, source) {
  const lines = text.trim().split(/\r?\n/);
  const headers = lines.shift().split(",");
  return lines.filter(Boolean).map((line, index) => {
    const values = line.split(",");
    if (values.length !== headers.length) {
      throw new Error(`${source}:${index + 2}: invalid column count`);
    }
    return Object.fromEntries(headers.map((header, position) => [header, values[position]]));
  });
}

function keyOf(item) {
  const box = item.bbox;
  return `${item.imageId}:${box.x}:${box.y}:${box.w}:${box.h}`;
}

async function api(pathname, options = {}) {
  let lastError;
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    try {
      const response = await fetch(`${apiUrl}${pathname}`, options);
      if (!response.ok) {
        const details = await response.text();
        throw new Error(`${response.status} ${response.statusText}: ${details}`);
      }
      if (response.status === 204) return null;
      return await response.json();
    } catch (error) {
      lastError = error;
      if (attempt < 3) await new Promise((resolve) => setTimeout(resolve, attempt * 1000));
    }
  }
  throw lastError;
}

async function existingObservationKeys() {
  const keys = new Set();
  let cursor;
  do {
    const params = new URLSearchParams({ limit: "100" });
    if (cursor) params.set("cursor", cursor);
    const page = await api(`/api/v1/gallery/observations?${params}`);
    for (const observation of page.items) {
      if (observation.image_id) {
        keys.add(keyOf({ imageId: observation.image_id, bbox: observation.bbox }));
      }
    }
    cursor = page.next_cursor;
  } while (cursor);
  return keys;
}

async function loadTasks() {
  const imageNames = await readdir(path.join(datasetDir, "images"));
  const imageFiles = new Map(
    imageNames
      .filter((name) => /^[a-f0-9]{32}\.jpg$/i.test(name))
      .map((name) => [path.parse(name).name.toLowerCase(), name]),
  );
  const tasks = [];
  const seen = new Set();

  for (const source of sources) {
    const rows = parseCsv(await readFile(path.join(datasetDir, source), "utf8"), source);
    for (const row of rows) {
      const imageId = row.image_id.toLowerCase();
      const fileName = imageFiles.get(imageId);
      if (!fileName) continue;
      const bbox = Object.fromEntries(["x", "y", "w", "h"].map((field) => [field, Number.parseInt(row[field], 10)]));
      const task = {
        imageId,
        imagePath: path.join(datasetDir, "images", fileName),
        bbox,
        vehicleId: row.vehicle_id || undefined,
        source,
      };
      const key = keyOf(task);
      if (!seen.has(key)) {
        seen.add(key);
        tasks.push(task);
      }
    }
  }
  return tasks;
}

async function importTask(task) {
  const bytes = await readFile(task.imagePath);
  const form = new FormData();
  form.append("image", new Blob([bytes], { type: "image/jpeg" }), `${task.imageId}.jpg`);
  const photo = await api("/api/v1/photos", { method: "POST", body: form });
  const body = {
    photo_id: photo.id,
    bbox: task.bbox,
    image_id: task.imageId,
    ...(task.vehicleId ? { vehicle_id: task.vehicleId } : {}),
  };
  return api("/api/v1/gallery/observations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

const tasks = await loadTasks();
const existing = await existingObservationKeys();
const allPending = tasks.filter((task) => !existing.has(keyOf(task)));
const pending = importLimit > 0 ? allPending.slice(0, importLimit) : allPending;
console.log(`Dataset: ${datasetDir}`);
console.log(`Annotated images available: ${tasks.length}`);
console.log(`Already in gallery: ${tasks.length - allPending.length}`);
console.log(`Pending import: ${allPending.length}${importLimit > 0 ? ` (processing first ${pending.length})` : ""}`);

let nextIndex = 0;
let completed = 0;
const failures = [];

async function worker() {
  while (true) {
    const index = nextIndex;
    nextIndex += 1;
    if (index >= pending.length) return;
    const task = pending[index];
    try {
      await importTask(task);
      completed += 1;
      if (completed % 10 === 0 || completed === pending.length) {
        console.log(`Progress: ${completed}/${pending.length}`);
      }
    } catch (error) {
      failures.push({ task, error });
      console.error(`FAILED ${task.source} ${task.imageId}: ${error.message}`);
    }
  }
}

await Promise.all(Array.from({ length: concurrency }, () => worker()));
console.log(`Imported: ${completed}`);
console.log(`Failed: ${failures.length}`);
if (failures.length) process.exitCode = 1;
