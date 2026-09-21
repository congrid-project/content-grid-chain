#!/usr/bin/env python3
"""Embedded into install.sh. Verify a trust anchor and supervise a state-sync node."""
import argparse
import base64
from datetime import datetime, timezone
import fcntl
import ipaddress
import json
import os
from pathlib import Path
import re
import signal
import socket
import subprocess
import sys
import time
from urllib.parse import urlencode, urlsplit

CHAIN_ID = "congrid-main"
UPGRADE_HEIGHT = 90000
TRUST_SECONDS = 86400


def rpc(base, method, **params):
    url = base.rstrip("/") + "/" + method
    if params:
        url += "?" + urlencode(params)
    response = json.loads(subprocess.check_output([
        "curl", "--fail", "--silent", "--show-error", "--connect-timeout", "5",
        "--max-time", "20", url,
    ]))
    if "error" in response:
        raise ValueError(f"{method}: {response['error']}")
    return response["result"]


def fields(data):
    """Decode the varint/length-delimited fields used by staking Query/Params."""
    index = 0

    def varint():
        nonlocal index
        value = 0
        for shift in range(0, 70, 7):
            byte = data[index]
            index += 1
            value |= (byte & 127) << shift
            if byte < 128:
                return value
        raise ValueError("Invalid protobuf varint")

    result = {}
    while index < len(data):
        tag = varint()
        wire = tag & 7
        if wire == 0:
            value = varint()
        elif wire == 2:
            size = varint()
            if index + size > len(data):
                raise ValueError("Truncated protobuf field")
            value = data[index:index + size]
            index += size
        else:
            raise ValueError(f"Unexpected protobuf wire type: {wire}")
        result[tag >> 3] = value
    return result


def trust_anchor(settings):
    if settings["chain_id"] != CHAIN_ID:
        raise ValueError("State-sync bootstrap supports congrid-main only")
    servers = [value.strip().rstrip("/") for value in settings["rpc_servers"]]
    if len(servers) != 2 or len(set(servers)) != 2:
        raise ValueError("Provide exactly two distinct state-sync RPC URLs")
    for server in servers:
        parsed = urlsplit(server)
        if (parsed.scheme not in ("https", "http") or not parsed.hostname
                or parsed.username or parsed.password or parsed.query or parsed.fragment):
            raise ValueError("State-sync RPC must be an HTTP(S) URL without credentials or query")
    statuses = [rpc(server, "status") for server in servers]
    ids = [status["node_info"]["id"] for status in statuses]
    if len(set(ids)) != 2 or any(not re.fullmatch(r"[0-9a-fA-F]{40}", value) for value in ids):
        raise ValueError("RPC URLs must identify two different nodes")
    for status in statuses:
        if status["node_info"]["network"] != CHAIN_ID or status["sync_info"]["catching_up"]:
            raise ValueError("RPC node has the wrong chain ID or is still catching up")
    height = min(int(s["sync_info"]["latest_block_height"]) for s in statuses) - 20
    if height <= UPGRADE_HEIGHT:
        raise ValueError("Trust height must be after ibc-transfer-v1@90000")
    blocks = [rpc(server, "block", height=str(height)) for server in servers]
    hashes = [block["block_id"]["hash"].upper() for block in blocks]
    if hashes[0] != hashes[1] or not re.fullmatch(r"[0-9A-F]{64}", hashes[0]):
        raise ValueError("The two RPC nodes disagree on the trusted block hash")
    for block in blocks:
        header = block["block"]["header"]
        if header["chain_id"] != CHAIN_ID or int(header["height"]) != height:
            raise ValueError("Incorrect trust block height or chain ID")
        stamp = re.sub(r"(\.\d{6})\d+", r"\1", header["time"]).replace("Z", "+00:00")
        age = (datetime.now(timezone.utc) - datetime.fromisoformat(stamp)).total_seconds()
        if not -60 <= age <= 3600:
            raise ValueError("Trusted block is stale; check the RPCs and local clock")
    for server in servers:
        response = rpc(server, "abci_query", path=json.dumps("/cosmos.staking.v1beta1.Query/Params"),
                       data="0x", height=str(height))["response"]
        if int(response.get("code", 0)):
            raise ValueError("Cannot verify the chain's unbonding period")
        params = fields(base64.b64decode(response["value"]))[1]
        duration = fields(params)[1]
        if fields(duration).get(1, 0) <= TRUST_SECONDS * 2:
            raise ValueError("Unbonding period is too short for the 24-hour trust period")

    peers = [p.strip() for p in settings.get("peers", "").split(",") if p.strip()]
    if not peers:
        for status in statuses:
            info = status["node_info"]
            address = re.sub(r"^tcp://", "", info["listen_addr"])
            host, port = address.rsplit(":", 1)
            try:
                if not ipaddress.ip_address(host.strip("[]")).is_global:
                    continue
            except ValueError:
                continue
            peers.append(info["id"] + "@" + address)
    if not peers:
        raise ValueError("Set persistent peers including a reachable snapshot provider")
    for peer in peers:
        if not re.fullmatch(r"[0-9a-fA-F]{40}@[^\s,]+:[0-9]+", peer):
            raise ValueError("Invalid state-sync persistent peer")
        address = peer.split("@", 1)[1]
        host, port = address.rsplit(":", 1)
        with socket.create_connection((host.strip("[]"), int(port)), timeout=5):
            pass
    return {"chain_id": CHAIN_ID, "rpc_servers": servers, "trust_height": height,
            "trust_hash": hashes[0], "trust_period": "24h0m0s", "peers": ",".join(dict.fromkeys(peers)),
            "verified_at": datetime.now(timezone.utc).isoformat()}


def atomic_json(path, value):
    path = Path(path)
    temporary = path.with_name(path.name + f".tmp.{os.getpid()}")
    temporary.write_text(json.dumps(value, indent=2) + "\n")
    temporary.chmod(0o640)
    os.replace(temporary, path)


def configure(home, anchor):
    path = Path(home) / "config/config.toml"
    text = path.read_text()
    updates = {"statesync": {"enable": "true", "rpc_servers": json.dumps(",".join(anchor["rpc_servers"])),
                             "trust_height": str(anchor["trust_height"]), "trust_hash": json.dumps(anchor["trust_hash"]),
                             "trust_period": json.dumps(anchor["trust_period"])},
               "p2p": {"persistent_peers": json.dumps(anchor["peers"])}}
    for section, values in updates.items():
        match = re.search(r"(?ms)^\[" + section + r"\][^\n]*\n(.*?)(?=^\[|\Z)", text)
        if not match:
            raise ValueError(f"Missing [{section}] configuration")
        body = match.group(1)
        for key, value in values.items():
            body, count = re.subn(r"(?m)^" + key + r"\s*=.*$", lambda _: f"{key} = {value}", body)
            if count != 1:
                raise ValueError(f"Expected exactly one [{section}].{key}")
        text = text[:match.start(1)] + body + text[match.end(1):]
    temporary = path.with_name(path.name + f".tmp.{os.getpid()}")
    temporary.write_text(text)
    temporary.chmod(path.stat().st_mode & 0o777)
    os.replace(temporary, path)
    atomic_json(Path(home) / "config/state-sync-trust.json", anchor)


def run_node(binary, home, settings):
    home = Path(home).resolve()
    # Keep this lock in the supervisor for the entire lifetime of the child.
    with (home / ".congrid-node.lock").open("a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ValueError("A managed node is already running for this home")
        # Do not start another process when an unmanaged daemon already owns RPC.
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", 26657))
        complete = home / "config/state-sync-complete.json"
        if complete.exists() and not all((home / "data" / name).is_dir()
                                         for name in ("application.db", "blockstore.db")):
            raise ValueError("Completed state-sync marker exists but chain databases are missing; use a new node home")
        if not complete.exists():
            configure(home, trust_anchor(settings))
        child = subprocess.Popen([binary, "start", "--home", str(home)])
        stopping = False

        def stop(signum, frame):
            nonlocal stopping
            stopping = True
            if child.poll() is None:
                child.terminate()

        signal.signal(signal.SIGTERM, stop)
        signal.signal(signal.SIGINT, stop)
        while child.poll() is None:
            if not stopping and not complete.exists():
                try:
                    status = rpc("http://127.0.0.1:26657", "status")
                    sync = status["sync_info"]
                    if (status["node_info"]["network"] == CHAIN_ID
                            and int(sync["latest_block_height"]) > UPGRADE_HEIGHT
                            and not sync["catching_up"]):
                        atomic_json(complete, {"chain_id": CHAIN_ID,
                                              "height": sync["latest_block_height"],
                                              "node_id": status["node_info"]["id"]})
                        print("State sync completed; subsequent restarts use local state", flush=True)
                except (OSError, ValueError, KeyError, subprocess.SubprocessError):
                    pass
            time.sleep(2)
        return child.returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["preflight", "configure", "run"])
    parser.add_argument("--settings", required=True)
    parser.add_argument("--output")
    parser.add_argument("--home")
    parser.add_argument("--binary")
    args = parser.parse_args()
    settings = json.loads(Path(args.settings).read_text())
    if args.action == "preflight":
        atomic_json(args.output, trust_anchor(settings))
    elif args.action == "configure":
        configure(args.home, settings)
    else:
        return run_node(args.binary, args.home, settings)
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, KeyError, IndexError, subprocess.SubprocessError) as error:
        print(f"State-sync bootstrap stopped: {error}", file=sys.stderr)
        sys.exit(1)
