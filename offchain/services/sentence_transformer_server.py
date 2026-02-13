#!/usr/bin/env python3
"""Minimal HTTP service exposing sentence-transformer embeddings.

The server loads a SentenceTransformer model at start-up and exposes:

- POST /embed : {"text": "...", "normalize": bool, "model": "optional"}
  Returns {"embedding": [...], "dim": int, "model": "..."}
- GET /healthz : readiness probe returning model metadata

Environment variables:
  SENTENCE_TRANSFORMER_MODEL   - model name (default: all-MiniLM-L6-v2)
  SENTENCE_TRANSFORMER_DEVICE  - inference device, e.g. cpu / cuda (default: cpu)

The listening host/port can be configured via CLI flags.
"""

from __future__ import annotations

import argparse
import json
import logging
import os
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Dict

try:
    from sentence_transformers import SentenceTransformer
except ImportError as exc:  # pragma: no cover - dependency handled by user environment
    raise SystemExit("sentence-transformers package is required: pip install sentence-transformers") from exc


LOGGER = logging.getLogger("sentence_transformer_service")


def load_model(name: str, device: str) -> SentenceTransformer:
    LOGGER.info("loading sentence transformer model", extra={"model": name, "device": device})
    model = SentenceTransformer(name, device=device)
    LOGGER.info(
        "model ready",
        extra={"model": name, "dim": model.get_sentence_embedding_dimension()},
    )
    return model


class EmbeddingHandler(BaseHTTPRequestHandler):
    server_version = "SentenceTransformerHTTP/1.0"

    def do_GET(self) -> None:  # noqa: N802 - signature required by BaseHTTPRequestHandler
        if self.path != "/healthz":
            self._write_response(HTTPStatus.NOT_FOUND, {"error": "unknown path"})
            return
        payload = {
            "model": self.server.model_name,
            "device": self.server.device,
            "dim": self.server.model.get_sentence_embedding_dimension(),
        }
        self._write_response(HTTPStatus.OK, payload)

    def do_POST(self) -> None:  # noqa: N802 - signature required by BaseHTTPRequestHandler
        if self.path != "/embed":
            self._write_response(HTTPStatus.NOT_FOUND, {"error": "unknown path"})
            return
        length_header = self.headers.get("Content-Length")
        if not length_header:
            self._write_response(HTTPStatus.BAD_REQUEST, {"error": "missing content-length"})
            return
        try:
            length = int(length_header)
        except ValueError:
            self._write_response(HTTPStatus.BAD_REQUEST, {"error": "invalid content-length"})
            return
        payload = self.rfile.read(length)
        try:
            request = json.loads(payload)
        except json.JSONDecodeError:
            self._write_response(HTTPStatus.BAD_REQUEST, {"error": "invalid json payload"})
            return

        text = request.get("text") or request.get("document")
        if not text or not isinstance(text, str):
            self._write_response(HTTPStatus.BAD_REQUEST, {"error": "text field required"})
            return

        normalize = bool(request.get("normalize", False))
        model_override = request.get("model")
        model = self.server.model
        if model_override and model_override != self.server.model_name:
            try:
                model = self.server.get_or_load_model(model_override)
            except Exception as exc:  # pragma: no cover - defensive logging
                LOGGER.exception("failed to load override model", extra={"model": model_override})
                self._write_response(HTTPStatus.INTERNAL_SERVER_ERROR, {"error": str(exc)})
                return

        embedding = model.encode(text, normalize_embeddings=normalize)
        vector = embedding.tolist() if hasattr(embedding, "tolist") else list(embedding)
        response = {
            "embedding": vector,
            "dim": len(vector),
            "model": model_override or self.server.model_name,
            "normalize": normalize,
        }
        self._write_response(HTTPStatus.OK, response)

    def log_message(self, fmt: str, *args: Any) -> None:  # noqa: D401 - keep default signature
        LOGGER.info("%s - %s", self.address_string(), fmt % args)

    def _write_response(self, status: HTTPStatus, payload: Dict[str, Any]) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status.value)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


class EmbeddingServer(ThreadingHTTPServer):
    def __init__(self, server_address, handler_class, model_name: str, device: str) -> None:  # type: ignore[override]
        super().__init__(server_address, handler_class)
        self.model_name = model_name
        self.device = device
        self.model = load_model(model_name, device)
        self._models: Dict[str, SentenceTransformer] = {model_name: self.model}

    def get_or_load_model(self, model_name: str) -> SentenceTransformer:
        if model_name in self._models:
            return self._models[model_name]
        model = load_model(model_name, self.device)
        self._models[model_name] = model
        return model


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Sentence Transformer HTTP Service")
    parser.add_argument("--host", default="0.0.0.0", help="listen host (default: %(default)s)")
    parser.add_argument("--port", type=int, default=9000, help="listen port (default: %(default)s)")
    parser.add_argument("--log-level", default="INFO", help="logging level")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    logging.basicConfig(level=getattr(logging, args.log_level.upper(), logging.INFO))

    model_name = os.getenv("SENTENCE_TRANSFORMER_MODEL", "all-MiniLM-L6-v2")
    device = os.getenv("SENTENCE_TRANSFORMER_DEVICE", "cpu")

    server = EmbeddingServer((args.host, args.port), EmbeddingHandler, model_name, device)
    LOGGER.info("listening", extra={"host": args.host, "port": args.port, "model": model_name})
    try:
        server.serve_forever()
    except KeyboardInterrupt:  # pragma: no cover - interactive usage
        LOGGER.info("shutting down on keyboard interrupt")
    finally:
        server.server_close()


if __name__ == "__main__":  # pragma: no cover
    main()
