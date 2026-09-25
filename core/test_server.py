import base64
import io
import json
import threading
import unittest
import urllib.error
import urllib.request

import numpy as np
from PIL import Image

from core.server import InputError, decode_input, make_server


class FakeExtractor:
    def extract_vector(self, pixels, bbox):
        vector = np.zeros(512, dtype=np.float32)
        vector[0] = 1
        return vector


class ServerTests(unittest.TestCase):
    def setUp(self):
        image = io.BytesIO()
        Image.new("RGB", (8, 8)).save(image, format="PNG")
        self.body = {"image": base64.b64encode(image.getvalue()).decode(),
                     "bbox": {"x": 0, "y": 0, "w": 8, "h": 8}}

    def test_bounds_and_input(self):
        pixels, bbox = decode_input(self.body)
        self.assertEqual(pixels.shape, (8, 8, 3))
        self.assertEqual(bbox, [0, 0, 8, 8])
        self.body["bbox"]["w"] = 9
        with self.assertRaises(InputError):
            decode_input(self.body)
        self.body["bbox"]["w"] = True
        with self.assertRaises(InputError):
            decode_input(self.body)

    def test_http_contract(self):
        server = make_server(FakeExtractor(), "test-v1", ("127.0.0.1", 0))
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        endpoint = "http://127.0.0.1:" + str(server.server_address[1])
        try:
            with urllib.request.urlopen(endpoint + "/readyz") as response:
                self.assertEqual(json.load(response)["model_version"], "test-v1")
            request = urllib.request.Request(endpoint + "/v1/features",
                data=json.dumps(self.body).encode(), headers={"Content-Type": "application/json"})
            with urllib.request.urlopen(request) as response:
                result = json.load(response)
            self.assertEqual(len(result["embedding"]), 512)
            self.assertEqual(result["embedding"][0], 1)
            self.assertEqual(result["model_version"], "test-v1")
            bad = urllib.request.Request(endpoint + "/v1/features", data=b"{",
                                         headers={"Content-Type": "application/json"})
            with self.assertRaises(urllib.error.HTTPError) as failure:
                urllib.request.urlopen(bad)
            self.assertEqual(failure.exception.code, 400)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


if __name__ == "__main__":
    unittest.main()
