#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
PROTO_DIR="$ROOT_DIR/proto"
REGISTRY_OUT="$ROOT_DIR/x/registry/typespb"
MINERS_OUT="$ROOT_DIR/x/miners/typespb"

rm -f "$REGISTRY_OUT"/*.pb.go
rm -f "$MINERS_OUT"/*.pb.go

cd "$PROTO_DIR"
# buf.gen.yaml outputs generated files to the repo root (..), with paths=import.
buf generate

