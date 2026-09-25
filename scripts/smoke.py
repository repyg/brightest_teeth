"""End-to-end check against a running API, using real ML and storage.

Usage: python scripts/smoke.py [http://localhost:8080]
Creates one small synthetic photo, removes its observations in finally.
The uploaded photo is retained (there is intentionally no photo deletion API).
This checks transport/integration, not ReID accuracy on real vehicles.
"""
import json
import math
import struct
import sys
import urllib.error
import urllib.request
import uuid
import zlib

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080").rstrip("/")


def request(method, path, body=None, content_type="application/json", status=200):
    if isinstance(body, dict):
        body = json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=body, method=method,
                                 headers={"Content-Type": content_type, "Origin": "http://localhost:5173"})
    try:
        response = urllib.request.urlopen(req, timeout=65)
    except urllib.error.HTTPError as exc:
        response = exc
    with response:
        raw = response.read()
        assert response.status == status, (method, path, response.status, raw)
        assert response.headers.get("Access-Control-Allow-Origin") == "http://localhost:5173"
        if "application/json" in response.headers.get("Content-Type", ""):
            return json.loads(raw)
        return raw


def test_png():
    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))
    rows = b"".join(b"\0" + bytes([x % 256, (x * 3) % 256, 128]) * 32 for x in range(32))
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 32, 32, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b"")


def main():
    request("GET", "/healthz")
    request("GET", "/readyz")
    spec = request("GET", "/openapi.json")
    assert spec["openapi"] == "3.0.3"
    request("OPTIONS", "/api/v1/search", status=204)
    image = test_png()
    boundary = "smoke" + uuid.uuid4().hex
    body = ("--" + boundary + '\r\nContent-Disposition: form-data; name="image"; filename="smoke.png"\r\nContent-Type: image/png\r\n\r\n').encode() + image + ("\r\n--" + boundary + "--\r\n").encode()
    photo = request("POST", "/api/v1/photos", body, "multipart/form-data; boundary=" + boundary, 201)
    photo_path = "/api/v1/photos/" + photo["id"]
    assert request("GET", photo_path)["width"] == 32
    assert request("GET", photo_path + "/content") == image
    query = {"photo_id": photo["id"], "bbox": {"x": 0, "y": 0, "w": 32, "h": 32}}
    feature = request("POST", "/api/v1/features", query)
    assert len(feature["embedding"]) == 512
    assert abs(math.sqrt(sum(x * x for x in feature["embedding"])) - 1) < 1e-5
    created = []
    try:
        for _ in range(2):
            observation = request("POST", "/api/v1/gallery/observations", {**query, "image_id": "smoke.png"}, status=201)
            created.append(observation["id"])
            assert request("GET", "/api/v1/gallery/observations/" + observation["id"])["photo_id"] == photo["id"]
        page = request("GET", "/api/v1/gallery/observations?limit=1")
        assert len(page["items"]) == 1 and page["next_cursor"]
        page2 = request("GET", "/api/v1/gallery/observations?limit=1&cursor=" + page["next_cursor"])
        assert page2["items"][0]["id"] != page["items"][0]["id"]
        matches = request("POST", "/api/v1/search", query)
        assert not matches["refused"] and len(matches["candidates"]) >= 2
        own_matches = [c for c in matches["candidates"] if c["observation"]["id"] in created]
        assert len(own_matches) == 2 and all(c["confidence"] > 0.999 for c in own_matches)
        excluded = request("POST", "/api/v1/search", {**query, "exclude_observation_ids": created})
        assert all(c["observation"]["id"] not in created for c in excluded["candidates"])
        ranking = request("POST", "/api/v1/search", {**query, "mode": "ranking", "top_k": 1})
        assert len(ranking["candidates"]) == 1 and ranking["threshold"] is None and not ranking["refused"]
        request("POST", "/api/v1/search", {**query, "bbox": {"x": 0, "y": 0, "w": 33, "h": 32}}, status=422)
        request("POST", "/api/v1/search", b"{", status=400)
        request("POST", "/api/v1/search", {**query, "mode": "ranking", "threshold": 0.5}, status=422)
        request("GET", "/api/v1/gallery/observations?cursor=invalid", status=400)
    finally:
        for observation_id in created:
            path = "/api/v1/gallery/observations/" + observation_id
            request("DELETE", path, status=204)
            request("DELETE", path, status=204)
            request("GET", path, status=404)
    print("PASS: all 12 API operations, real ML inference, PostgreSQL/pgvector, MinIO, CORS, validation and cleanup")
    print("Retained smoke photo:", photo["id"])


if __name__ == "__main__":
    main()
