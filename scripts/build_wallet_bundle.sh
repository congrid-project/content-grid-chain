#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_FILE="$ROOT_DIR/cmd/congrid-site/static/wallet-deps.bundle.mjs"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

cd "$TMP_DIR"

npm init -y >/dev/null 2>&1
npm install --no-save --loglevel=error \
  esbuild \
  @cosmjs/stargate@0.32.4 \
  @cosmjs/proto-signing@0.32.4 \
  long@5.2.3 \
  protobufjs@7.3.0 >/dev/null

cat > entry.mjs <<'EOF'
import { SigningStargateClient, GasPrice, calculateFee, defaultRegistryTypes } from "@cosmjs/stargate";
import { Registry } from "@cosmjs/proto-signing";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long;
  _m0.configure();
}

export { SigningStargateClient, GasPrice, calculateFee, defaultRegistryTypes, Registry, Long, _m0 };
EOF

npx esbuild entry.mjs \
  --bundle \
  --platform=browser \
  --format=esm \
  --external:crypto \
  --minify \
  --legal-comments=none \
  --outfile="$OUT_FILE"

printf 'wrote %s\n' "$OUT_FILE"
