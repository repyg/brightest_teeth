"""Private HTTP transport; contract: backend/internal/mlapi/openapi.yaml."""

import base64
import binascii
import io
import json
import logging
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import numpy as np
from PIL import Image, UnidentifiedImageError

MAX_BODY = 15 * 1024 * 1024
MAX_IMAGE = 10 * 1024 * 1024
MAX_PIXELS = 40_000_000
Image.MAX_IMAGE_PIXELS = MAX_PIXELS


class InputError(Exception):
    def __init__(self, status, code, message):
        super().__init__(message)
        self.status, self.code = status, code


def decode_input(body):
    if not isinstance(body, dict) or set(body) != {"image", "bbox"}:
        raise InputError(422, "validation_error", "image and bbox are required")
    bbox = body["bbox"]
    if not isinstance(bbox, dict) or set(bbox) != {"x", "y", "w", "h"}:
        raise InputError(422, "validation_error", "Invalid bbox")
    if any(type(bbox[k]) is not int for k in bbox):
        raise InputError(422, "validation_error", "BBox coordinates must be integers")
    if not isinstance(body["image"], str):
        raise InputError(422, "validation_error", "image must be base64")
    try:
        raw = base64.b64decode(body["image"], validate=True)
    except (ValueError, binascii.Error):
        raise InputError(422, "validation_error", "Invalid base64") from None
    if len(raw) > MAX_IMAGE:
        raise InputError(413, "payload_too_large", "Image exceeds 10 MiB")
    try:
        with Image.open(io.BytesIO(raw)) as image:
            if image.format not in ("JPEG", "PNG"):
                raise InputError(415, "unsupported_media_type", "Expected JPEG or PNG")
            width, height = image.size
            if width * height > MAX_PIXELS:
                raise InputError(422, "validation_error", "Too many pixels")
            x, y, w, h = (bbox[k] for k in ("x", "y", "w", "h"))
            if x < 0 or y < 0 or w < 1 or h < 1 or x + w > width or y + h > height:
                raise InputError(422, "validation_error", "BBox outside image")
            # No EXIF transposition: coordinates refer to the encoded matrix.
            pixels = np.array(image.convert("RGB"))
    except (UnidentifiedImageError, OSError, ValueError, Image.DecompressionBombError):
        raise InputError(422, "validation_error", "Invalid image") from None
    return pixels, [x, y, w, h]


def make_server(extractor, version, address):
    inference_lock = threading.Lock()

    class Handler(BaseHTTPRequestHandler):
        def setup(self):
            super().setup()
            self.connection.settimeout(60)

        def reply(self, status, body):
            data = json.dumps(body, allow_nan=False).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            if self.path == "/readyz":
                self.reply(200, {"model_version": version})
            else:
                self.reply(404, {"code": "not_found", "message": "Not found"})

        def do_POST(self):
            if self.path != "/v1/features":
                self.reply(404, {"code": "not_found", "message": "Not found"})
                return
            if not inference_lock.acquire(blocking=False):
                self.reply(503, {"code": "dependency_unavailable", "message": "Worker busy"})
                return
            try:
                if self.headers.get_content_type() != "application/json":
                    raise InputError(415, "unsupported_media_type", "Expected application/json")
                try:
                    size = int(self.headers.get("Content-Length", "0"))
                except ValueError:
                    raise InputError(400, "bad_request", "Invalid Content-Length") from None
                if size > MAX_BODY:
                    raise InputError(413, "payload_too_large", "Request too large")
                if size <= 0:
                    raise InputError(400, "bad_request", "Empty request")
                try:
                    body = json.loads(self.rfile.read(size))
                except (ValueError, UnicodeDecodeError):
                    raise InputError(400, "bad_request", "Invalid JSON") from None
                pixels, bbox = decode_input(body)
                vector = np.asarray(extractor.extract_vector(pixels, bbox), dtype=np.float32)
                if vector.shape != (512,) or not np.isfinite(vector).all() or abs(float(np.linalg.norm(vector)) - 1) > 1e-5:
                    raise RuntimeError("Invalid model embedding")
                self.reply(200, {"embedding": vector.tolist(), "model_version": version})
            except InputError as exc:
                self.reply(exc.status, {"code": exc.code, "message": str(exc)})
            except Exception:
                logging.exception("Inference failed")
                self.reply(503, {"code": "dependency_unavailable", "message": "Inference failed"})
            finally:
                inference_lock.release()

    return ThreadingHTTPServer(address, Handler)


def main():
    # Import/load once at startup. No socket is opened until weights are ready.
    from .feature_extractor import VehicleFeatureExtractor

    logging.basicConfig(level=logging.INFO)
    version = os.getenv("MODEL_VERSION", "reid-resnet50-v1")
    extractor = VehicleFeatureExtractor(
        os.getenv("MODEL_WEIGHTS", "/app/core/weights/reid_model_final.pth"),
        device=os.getenv("MODEL_DEVICE", "cpu"),
    )
    server = make_server(extractor, version, ("0.0.0.0", int(os.getenv("ML_PORT", "8000"))))
    logging.info("Model %s loaded; listening on %s", version, server.server_address)
    server.serve_forever()


if __name__ == "__main__":
    main()
