#!/usr/bin/env python3
"""Exercise two isolated Congrid daemons and Hermes, using test funds only.

Requires Python 3.8+, a built content-grid-d and Hermes 1.13.3. Optionally
upgrade chain A from a pre-IBC binary before opening the transfer channel.
All endpoints are loopback and all homes/keys are created in a fresh directory.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.request


def wait_for(description, check, seconds=90):
    deadline = time.monotonic() + seconds
    last_error = None
    while time.monotonic() < deadline:
        try:
            result = check()
            if result:
                return result
        except (OSError, ValueError, KeyError) as exc:
            last_error = exc
        time.sleep(0.5)
    raise RuntimeError(f"Timed out: {description}; last error: {last_error}")


def get_json(url):
    with urllib.request.urlopen(url, timeout=3) as response:
        return json.load(response)


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


class Localnet:
    def __init__(self, args):
        self.binary = str(Path(args.binary).resolve())
        self.hermes = str(Path(args.hermes).resolve())
        self.old_binary = str(Path(args.upgrade_from).resolve()) if args.upgrade_from else None
        self.root = Path(tempfile.mkdtemp(prefix="congrid-ibc-", dir=args.work_parent)).resolve()
        self.processes = []
        self.files = []
        self.chains = []
        self.tx_hashes = []
        self.report = {"real_funds_used": False, "counterparty": "local Congrid daemon", "checks": []}

    def run(self, command, secret=False, timeout=90):
        result = subprocess.run(command, capture_output=True, text=True, timeout=timeout)
        if not secret:
            with (self.root / "commands.log").open("a") as log:
                log.write(json.dumps(command) + "\n" + result.stdout + result.stderr + "\n")
        if result.returncode:
            detail = "key command failed (output withheld)" if secret else result.stderr[-3000:]
            raise RuntimeError(f"Command failed: {command[1:3]}: {detail}")
        return result.stdout

    def cli(self, chain, *args, secret=False):
        return self.run([chain["binary"], *map(str, args), "--home", str(chain["home"])], secret=secret)

    def spawn(self, command, log_name):
        log = (self.root / log_name).open("a")
        self.files.append(log)
        process = subprocess.Popen(command, stdout=log, stderr=subprocess.STDOUT)
        self.processes.append(process)
        return process

    @staticmethod
    def stop(process):
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)

    def close(self):
        for process in reversed(self.processes):
            self.stop(process)
        for log in self.files:
            log.close()

    def start_chain(self, chain):
        chain["process"] = self.spawn([
            chain["binary"], "start", "--home", str(chain["home"]),
            "--rpc.laddr", chain["rpc"].replace("http:", "tcp:"),
            "--p2p.laddr", f"tcp://127.0.0.1:{chain['p2p']}",
            "--grpc.address", f"127.0.0.1:{chain['grpc']}",
            "--api.address", chain["rest"].replace("http:", "tcp:"),
            "--grpc-web.enable=false", "--minimum-gas-prices", "0.001ucongrid",
        ], chain["id"] + ".log")
        wait_for("chain producing blocks", lambda: self.height(chain) >= 2)

    @staticmethod
    def height(chain):
        return int(get_json(chain["rpc"] + "/status")["result"]["sync_info"]["latest_block_height"])

    def setup_chain(self, index):
        chain = {
            "id": f"congrid-ibc-local-{index}", "home": self.root / f"chain-{index}",
            "binary": self.old_binary if index == 1 and self.old_binary else self.binary,
            "rpc": f"http://127.0.0.1:{free_port()}", "rest": f"http://127.0.0.1:{free_port()}",
            "grpc": free_port(), "p2p": free_port(),
        }
        self.cli(chain, "devnet", "--chain-id", chain["id"], "--amount", "10000000000000ucongrid", secret=True)
        for name in ("relayer", "trader"):
            key = json.loads(self.cli(chain, "keys", "add", name, "--keyring-backend", "test", "--output", "json", secret=True))
            chain[name] = key["address"]
            self.cli(chain, "genesis", "add-genesis-account", key["address"], "1000000000000ucongrid,1000000000000uusdc-test")
            if name == "relayer":
                chain["mnemonic"] = self.root / f"relayer-{index}.txt"
                chain["mnemonic"].write_text(key["mnemonic"] + "\n")
                chain["mnemonic"].chmod(0o600)
        genesis_path = chain["home"] / "config/genesis.json"
        genesis = json.loads(genesis_path.read_text())
        gov = genesis["app_state"]["gov"]["params"]
        gov.update(voting_period="8s", expedited_voting_period="4s", max_deposit_period="60s",
                   min_deposit=[{"denom": "ucongrid", "amount": "1000"}])
        genesis_path.write_text(json.dumps(genesis))
        config = chain["home"] / "config/config.toml"
        config.write_text(re.sub(r'^timeout_commit = .*$', 'timeout_commit = "1s"', config.read_text(), flags=re.M))
        self.chains.append(chain)
        self.start_chain(chain)
        return chain

    def tx(self, chain, *args, key="trader"):
        result = json.loads(self.cli(chain, "tx", *args, "--from", key, "--keyring-backend", "test",
                                    "--chain-id", chain["id"], "--node", chain["rpc"], "--gas", "1000000",
                                    "--fees", "1000ucongrid", "--yes", "--output", "json"))
        if int(result.get("code", 0)):
            raise RuntimeError(f"CheckTx failed: {result}")
        tx_hash = result["txhash"]
        receipt = wait_for("transaction inclusion", lambda: get_json(
            chain["rpc"] + f'/tx?hash=0x{tx_hash}')["result"])
        if int(receipt["tx_result"].get("code", 0)):
            raise RuntimeError(f"DeliverTx failed: {receipt}")
        self.tx_hashes.append({"chain": chain["id"], "hash": tx_hash})
        return receipt

    @staticmethod
    def balances(chain, address=None):
        data = get_json(chain["rest"] + "/cosmos/bank/v1beta1/balances/" + (address or chain["trader"]))
        return {coin["denom"]: int(coin["amount"]) for coin in data["balances"]}

    def upgrade(self, chain):
        print("Testing pre-IBC database upgrade through governance...", flush=True)
        original = self.balances(chain)
        genesis_hash = hashlib.sha256((chain["home"] / "config/genesis.json").read_bytes()).hexdigest()
        account = json.loads(self.cli(chain, "query", "auth", "module-account", "gov", "--node", chain["rpc"], "--output", "json"))["account"]
        authority = account.get("base_account", account.get("value", account))["address"]
        height = self.height(chain) + 35
        proposal = self.root / "upgrade-proposal.json"
        proposal.write_text(json.dumps({
            "messages": [{"@type": "/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade", "authority": authority,
                          "plan": {"name": "ibc-transfer-v1", "height": str(height), "info": "local IBC rehearsal"}}],
            "metadata": "", "deposit": "1000ucongrid", "title": "Local IBC upgrade",
            "summary": "Initialize IBC stores on a disposable chain", "expedited": False,
        }))
        self.tx(chain, "gov", "submit-proposal", str(proposal), key="validator")
        self.tx(chain, "gov", "vote", "1", "yes", key="validator")
        upgrade_info = chain["home"] / "data/upgrade-info.json"
        wait_for("old binary reaches upgrade halt", lambda: upgrade_info.exists(), seconds=120)
        # CometBFT can stop consensus while its RPC process stays alive.
        self.stop(chain["process"])
        info = json.loads(upgrade_info.read_text())
        assert info["name"] == "ibc-transfer-v1" and int(info["height"]) == height, info
        chain["binary"] = self.binary
        self.start_chain(chain)
        wait_for("upgraded chain passes upgrade height", lambda: self.height(chain) > height)
        assert self.balances(chain) == original, "Upgrade changed trader balances"
        assert hashlib.sha256((chain["home"] / "config/genesis.json").read_bytes()).hexdigest() == genesis_hash
        done = get_json(chain["rest"] + "/cosmos/upgrade/v1beta1/applied_plan/ibc-transfer-v1")
        assert int(done["height"]) == height, done
        self.report["checks"].append("pre-IBC governance upgrade preserves balances and genesis")

    def setup_hermes(self):
        text = """[global]
log_level = 'info'
[mode.clients]
enabled = true
refresh = true
misbehaviour = true
[mode.connections]
enabled = false
[mode.channels]
enabled = false
[mode.packets]
enabled = true
clear_interval = 10
clear_on_start = true
tx_confirmation = true
"""
        for chain in self.chains:
            text += f"""
[[chains]]
id = '{chain['id']}'
type = 'CosmosSdk'
rpc_addr = '{chain['rpc']}'
grpc_addr = 'http://127.0.0.1:{chain['grpc']}'
event_source = {{ mode = 'push', url = '{chain['rpc'].replace('http:', 'ws:')}/websocket', batch_delay = '200ms' }}
rpc_timeout = '10s'
trusted_node = true
account_prefix = 'congrid'
key_name = 'relayer'
key_store_folder = '{self.root / 'hermes-keys'}'
store_prefix = 'ibc'
default_gas = 300000
max_gas = 3000000
gas_price = {{ price = 0.001, denom = 'ucongrid' }}
gas_multiplier = 1.3
max_msg_num = 10
max_tx_size = 180000
clock_drift = '5s'
max_block_time = '5s'
trusting_period = '14days'
trust_threshold = {{ numerator = '1', denominator = '3' }}
address_type = {{ derivation = 'cosmos' }}
compat_mode = '0.38'
[chains.packet_filter]
policy = 'allow'
list = [['transfer', '*']]
"""
        self.config = self.root / "hermes.toml"
        self.config.write_text(text)
        self.hermes_cmd = [self.hermes, "--config", str(self.config)]
        for chain in self.chains:
            self.run([*self.hermes_cmd, "keys", "add", "--chain", chain["id"], "--mnemonic-file", str(chain["mnemonic"])], secret=True)
            chain["mnemonic"].unlink()
        self.run([*self.hermes_cmd, "health-check"])
        self.run([*self.hermes_cmd, "create", "channel", "--a-chain", self.chains[0]["id"],
                  "--b-chain", self.chains[1]["id"], "--a-port", "transfer", "--b-port", "transfer",
                  "--order", "unordered", "--channel-version", "ics20-1", "--new-client-connection", "--yes"], timeout=180)
        for chain in self.chains:
            channels = get_json(chain["rest"] + "/ibc/core/channel/v1/channels")["channels"]
            assert len(channels) == 1 and channels[0]["state"] == "STATE_OPEN", channels
            chain["channel"] = channels[0]["channel_id"]
        self.report["checks"].append("Hermes opens Tendermint clients, connection and ICS-20 channel")
        self.relayer = self.spawn([*self.hermes_cmd, "start"], "hermes.log")

    def transfer(self, source, dest, amount, denom, timeout_seconds=600, receiver=None):
        return self.tx(source, "ibc-transfer", "transfer", "transfer", source["channel"],
                       receiver or dest["trader"], f"{amount}{denom}",
                       "--packet-timeout-timestamp", str(timeout_seconds * 1_000_000_000))

    @staticmethod
    def voucher(chain, denom):
        trace = f"transfer/{chain['channel']}/{denom}"
        return "ibc/" + hashlib.sha256(trace.encode()).hexdigest().upper()

    def no_pending(self, chain):
        return not get_json(chain["rest"] + f"/ibc/core/channel/v1/channels/{chain['channel']}/ports/transfer/packet_commitments")["commitments"]

    def check_roundtrip(self, source, dest, amount, denom):
        initial = self.balances(source).get(denom, 0)
        voucher = self.voucher(dest, denom)
        self.transfer(source, dest, amount, denom)
        wait_for("voucher received", lambda: self.balances(dest).get(voucher, 0) == amount)
        wait_for("outbound acknowledgement", lambda: self.no_pending(source))
        self.transfer(dest, source, amount, voucher)
        expected = initial - (1000 if denom == "ucongrid" else 0)
        wait_for("native tokens returned", lambda: self.balances(source).get(denom, 0) == expected)
        wait_for("return acknowledgement", lambda: self.no_pending(dest))
        assert self.balances(dest).get(voucher, 0) == 0
        self.report["checks"].append(f"{amount}{denom} round trip, voucher burned, acknowledgements cleared")

    def check_refunds(self, source, dest):
        initial = self.balances(source)["ucongrid"]
        self.transfer(source, dest, 1_000_000, "ucongrid", receiver="invalid-receiver")
        wait_for("error acknowledgement refund", lambda: self.balances(source)["ucongrid"] == initial - 1000)
        wait_for("error acknowledgement clears commitment", lambda: self.no_pending(source))
        self.report["checks"].append("invalid receiver refunded through error acknowledgement")

        self.stop(self.relayer)
        initial = self.balances(source)["ucongrid"]
        self.transfer(source, dest, 2_000_000, "ucongrid", timeout_seconds=15)
        assert self.balances(source)["ucongrid"] == initial - 2_001_000
        # Let the destination commit a time beyond the packet deadline while
        # the relayer is stopped, then require Hermes to relay the refund.
        deadline = time.time() + 20
        wait_for("packet deadline passes", lambda: time.time() > deadline, seconds=30)
        self.relayer = self.spawn([*self.hermes_cmd, "start"], "hermes.log")
        wait_for("timeout refund", lambda: self.balances(source)["ucongrid"] == initial - 1000)
        wait_for("timeout clears commitment", lambda: self.no_pending(source))
        self.report["checks"].append("relayer outage followed by proof-based timeout refund")

    def test(self):
        print(f"Local test homes and logs: {self.root}", flush=True)
        source, dest = self.setup_chain(1), self.setup_chain(2)
        if self.old_binary:
            self.upgrade(source)
        print("Opening IBC channel with Hermes...", flush=True)
        self.setup_hermes()
        print("Testing CONGRID and 1,000 test USDC round trips...", flush=True)
        self.check_roundtrip(source, dest, 10_000_000, "ucongrid")
        self.check_roundtrip(dest, source, 1_000_000_000, "uusdc-test")
        print("Testing error and timeout refunds...", flush=True)
        self.check_refunds(source, dest)
        self.report.update(status="passed", transactions=self.tx_hashes,
                           channels=[{"chain": c["id"], "channel": c["channel"]} for c in self.chains])
        (self.root / "report.json").write_text(json.dumps(self.report, indent=2) + "\n")
        print(json.dumps(self.report, indent=2), flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", default="./content-grid-d")
    parser.add_argument("--hermes", default=shutil.which("hermes"))
    parser.add_argument("--upgrade-from", help="Pre-IBC daemon binary for the upgrade rehearsal")
    parser.add_argument("--work-parent", help="Parent for a NEW temporary test directory")
    args = parser.parse_args()
    for path in (args.binary, args.hermes, args.upgrade_from):
        if path and not os.access(path, os.X_OK):
            parser.error(f"Not an executable: {path}")
    if not args.hermes:
        parser.error("Install Hermes or pass --hermes /path/to/hermes")
    # Disposable test keys and state are readable only by the current user.
    os.umask(0o077)
    net = Localnet(args)
    try:
        net.test()
    finally:
        net.close()


if __name__ == "__main__":
    main()
