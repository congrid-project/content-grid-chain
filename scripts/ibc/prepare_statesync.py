#!/usr/bin/env python3
"""Prepare (but do not start) an isolated post-IBC mainnet state-sync node.

Requires an RPC tunnel to val2 on localhost:28657, curl, Python 3.8+, and the
native release bundle for this host. Prints the new test directory to stdout.
No existing node directory is opened and no validator keys are imported.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import socket
import subprocess
import sys
import tarfile
import tempfile
from datetime import datetime, timezone
from urllib.parse import urlencode


CHAIN_ID = "congrid-main"
SOURCE_COMMIT = "5907b65faa971afd9e0c29e74284baa03675e37c"
GENESIS_SHA = "779abab0b56bf4b0edf6951cf63f1cc950d2ba1faee9317b41205400ee7481d2"
PORTS = (27656, 27657, 27658)


def download(url):
    return subprocess.check_output([
        "curl", "-fsS", "--location", "--connect-timeout", "5",
        "--max-time", "30", url,
    ])


def rpc(base, method, **params):
    query = "?" + urlencode(params) if params else ""
    response = json.loads(download(base.rstrip("/") + "/" + method + query))
    if "error" in response:
        raise ValueError(f"RPC {method}: {response['error']}")
    return response["result"]


def patch_toml(path, updates):
    text = path.read_text()
    for section, values in updates.items():
        match = re.search(
            r"(?ms)^\[" + re.escape(section) + r"\][^\n]*\n(.*?)(?=^\[|\Z)", text
        )
        if not match:
            raise ValueError(f"Missing [{section}] in {path}")
        body = match.group(1)
        for key, value in values.items():
            body, count = re.subn(
                r"(?m)^" + re.escape(key) + r"\s*=.*$",
                lambda _: f"{key} = {value}", body,
            )
            if count != 1:
                raise ValueError(f"Expected one [{section}].{key} in {path}")
        text = text[:match.start(1)] + body + text[match.end(1):]
    path.write_text(text)


def prepare(args):
    system = {"Darwin": "darwin", "Linux": "linux"}[platform.system()]
    arch = {"x86_64": "amd64", "arm64": "arm64", "aarch64": "arm64"}[platform.machine()]
    bundle = args.bundle or (
        Path.home() / "congrid-releases/native-ibc-transfer-v1-r1"
        / f"congrid-native-{system}-{arch}.tar.gz"
    )
    expected = Path(str(bundle) + ".sha256").read_text().split()[0].lower()
    digest = hashlib.sha256()
    with bundle.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    if digest.hexdigest() != expected:
        raise ValueError("Native bundle SHA-256 mismatch")

    for port in PORTS:
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", port))

    endpoints = [args.primary_rpc, args.witness_rpc]
    statuses = [rpc(url, "status") for url in endpoints]
    if len({s["node_info"]["id"] for s in statuses}) != 2:
        raise ValueError("RPCs must belong to two different nodes")
    for status in statuses:
        if status["node_info"]["network"] != CHAIN_ID or status["sync_info"]["catching_up"]:
            raise ValueError("RPC node has wrong chain ID or is still catching up")
    height = min(int(s["sync_info"]["latest_block_height"]) for s in statuses) - 20
    if height <= 90000:
        raise ValueError("Trust height must be after the IBC upgrade")
    blocks = [rpc(url, "block", height=str(height)) for url in endpoints]
    hashes = [b["block_id"]["hash"] for b in blocks]
    if hashes[0] != hashes[1] or not re.fullmatch(r"[0-9A-Fa-f]{64}", hashes[0]):
        raise ValueError("Independent RPCs disagree on the trusted block hash")
    for block in blocks:
        header = block["block"]["header"]
        if int(header["height"]) != height or header["chain_id"] != CHAIN_ID:
            raise ValueError("Unexpected trusted block header")
        stamp = re.sub(r"(\.\d{6})\d+", r"\1", header["time"]).replace("Z", "+00:00")
        age = (datetime.now(timezone.utc) - datetime.fromisoformat(stamp)).total_seconds()
        if not -60 <= age <= 3600:
            raise ValueError("Trusted block is stale or local clock is incorrect")

    peers = []
    for status in statuses:
        info = status["node_info"]
        address = info["listen_addr"].removeprefix("tcp://") if sys.version_info >= (3, 9) else info["listen_addr"].replace("tcp://", "", 1)
        host, port = address.rsplit(":", 1)
        if host in ("0.0.0.0", "127.0.0.1", "localhost"):
            raise ValueError("RPC node does not advertise a public P2P address")
        with socket.create_connection((host, int(port)), timeout=5):
            pass
        peers.append(info["id"] + "@" + address)

    genesis = download("https://congrid.net/downloads/genesis.json")
    if hashlib.sha256(genesis).hexdigest() != GENESIS_SHA:
        raise ValueError("Genesis differs from the verified mainnet genesis")
    if json.loads(genesis)["chain_id"] != CHAIN_ID:
        raise ValueError("Genesis chain ID mismatch")

    args.work_parent.mkdir(parents=True, exist_ok=True)
    root = Path(tempfile.mkdtemp(prefix="test-", dir=args.work_parent)).resolve()
    print(f"Preparing {root}", file=sys.stderr)
    (root / "bin").mkdir()
    binary = root / "bin/content-grid-d"
    with tarfile.open(bundle, "r:gz") as archive:
        info = archive.extractfile("congrid-native/BUILD-INFO").read().decode()
        metadata = dict(line.split("=", 1) for line in info.splitlines() if "=" in line)
        if (metadata.get("source_commit") != SOURCE_COMMIT
                or metadata.get("source_version") != "ibc-transfer-v1"
                or metadata.get("target") != f"{system}/{arch}"):
            raise ValueError("Unexpected release source, version, or target")
        member = archive.getmember("congrid-native/bin/content-grid-d")
        if not member.isfile():
            raise ValueError("Node binary is not a regular archive member")
        binary.write_bytes(archive.extractfile(member).read())
    binary.chmod(0o755)
    version = subprocess.check_output([str(binary), "version"], text=True).strip()
    if version != "ibc-transfer-v1":
        raise ValueError(f"Unexpected binary version: {version}")
    home = root / "node"
    with (root / "init.log").open("w") as log:
        subprocess.run([
            str(binary), "init", "statesync-rehearsal", "--chain-id", CHAIN_ID,
            "--home", str(home),
        ], cwd=root, stdout=log, stderr=subprocess.STDOUT, check=True)
    (home / "config/genesis.json").write_bytes(genesis)
    patch_toml(home / "config/config.toml", {
        "rpc": {"laddr": '"tcp://127.0.0.1:27657"', "pprof_laddr": '""'},
        "p2p": {"laddr": '"tcp://127.0.0.1:27656"', "external_address": '""',
                "persistent_peers": json.dumps(",".join(peers)), "seeds": '""',
                "pex": "false"},
        "statesync": {"enable": "true", "rpc_servers": json.dumps(",".join(endpoints)),
                      "trust_height": str(height), "trust_hash": json.dumps(hashes[0]),
                      "trust_period": '"24h0m0s"'},
        "instrumentation": {"prometheus": "false"},
    })
    patch_toml(home / "config/app.toml", {
        "api": {"enable": "false"}, "grpc": {"enable": "false"},
        "grpc-web": {"enable": "false"},
    })
    # Set the top-level ABCI address, even though the normal ABCI client is local.
    config = home / "config/config.toml"
    text, count = re.subn(r'(?m)^proxy_app\s*=.*$',
                         'proxy_app = "tcp://127.0.0.1:27658"', config.read_text())
    if count != 1:
        raise ValueError("Expected one proxy_app setting")
    config.write_text(text)
    report = {
        "prepared_at": datetime.now(timezone.utc).isoformat(),
        "chain_id": CHAIN_ID, "trust_height": height, "trust_hash": hashes[0],
        "rpc_servers": endpoints, "peers": peers, "bundle_sha256": expected,
        "source_commit": SOURCE_COMMIT, "trust_period": "24h",
        "status": "prepared_only_not_started_or_verified",
    }
    (root / "preparation.json").write_text(json.dumps(report, indent=2) + "\n")
    print(f"Verified trust height {height} on two nodes; node NOT started.", file=sys.stderr)
    return root


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", type=Path)
    parser.add_argument("--primary-rpc", default="http://127.0.0.1:28657")
    parser.add_argument("--witness-rpc", default="https://congrid.net/rpc")
    parser.add_argument("--work-parent", type=Path,
                        default=Path.home() / "congrid-statesync-tests")
    args = parser.parse_args()
    os.umask(0o077)
    print(prepare(args))


if __name__ == "__main__":
    main()
