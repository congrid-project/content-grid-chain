import base64
import copy
from datetime import datetime, timedelta, timezone
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch, MagicMock

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location("state_sync", ROOT / "scripts/installer/state_sync.py")
sync = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sync)


def varint(value):
    result = bytearray()
    while value > 127:
        result.append((value & 127) | 128)
        value >>= 7
    return bytes(result + bytes([value]))


def nested(value):
    return b"\x0a" + varint(len(value)) + value


class TrustTests(unittest.TestCase):
    def setUp(self):
        self.settings = {"chain_id": "congrid-main", "rpc_servers": ["https://one/rpc", "https://two/rpc"]}
        self.statuses = [{"node_info": {"id": c * 40, "network": "congrid-main", "listen_addr": "8.8.8.8:26656"},
                          "sync_info": {"latest_block_height": "105600", "catching_up": False}} for c in ("a", "b")]
        header = {"height": "105580", "chain_id": "congrid-main",
                  "time": datetime.now(timezone.utc).isoformat()}
        self.blocks = [{"block_id": {"hash": "A" * 64}, "block": {"header": copy.deepcopy(header)}} for _ in range(2)]
        self.unbonding = 21 * 86400
        self.code = 0
        self.rpc_patch = patch.object(sync, "rpc", self.rpc)
        self.rpc_patch.start()
        self.connect_patch = patch.object(sync.socket, "create_connection", return_value=MagicMock())
        self.connect_patch.start()
        self.addCleanup(self.rpc_patch.stop)
        self.addCleanup(self.connect_patch.stop)

    def rpc(self, base, method, **params):
        index = self.settings["rpc_servers"].index(base)
        if method == "status":
            return self.statuses[index]
        if method == "block":
            return self.blocks[index]
        self.assertEqual(params["height"], "105580")
        duration = b"\x08" + varint(self.unbonding)
        return {"response": {"code": self.code, "value": base64.b64encode(nested(nested(duration))).decode()}}

    def test_valid_anchor(self):
        anchor = sync.trust_anchor(self.settings)
        self.assertEqual(anchor["trust_height"], 105580)
        self.assertEqual(anchor["trust_period"], "24h0m0s")
        self.assertIn("a" * 40 + "@8.8.8.8:26656", anchor["peers"])

    def test_conflicting_hashes(self):
        self.blocks[1]["block_id"]["hash"] = "B" * 64
        with self.assertRaisesRegex(ValueError, "disagree"):
            sync.trust_anchor(self.settings)

    def test_same_node_different_urls(self):
        self.statuses[1]["node_info"]["id"] = "a" * 40
        with self.assertRaisesRegex(ValueError, "different nodes"):
            sync.trust_anchor(self.settings)

    def test_wrong_chain_or_syncing(self):
        for field, value in (("network", "other-chain"), ("catching_up", True)):
            with self.subTest(field=field):
                section = "node_info" if field == "network" else "sync_info"
                before = self.statuses[0][section][field]
                self.statuses[0][section][field] = value
                with self.assertRaises(ValueError):
                    sync.trust_anchor(self.settings)
                self.statuses[0][section][field] = before

    def test_stale_or_future_trust(self):
        for hours in (-2, 2):
            with self.subTest(hours=hours):
                self.blocks[0]["block"]["header"]["time"] = (datetime.now(timezone.utc) + timedelta(hours=hours)).isoformat()
                with self.assertRaisesRegex(ValueError, "stale"):
                    sync.trust_anchor(self.settings)

    def test_insufficient_unbonding(self):
        self.unbonding = 86400
        with self.assertRaisesRegex(ValueError, "Unbonding"):
            sync.trust_anchor(self.settings)

    def test_query_error(self):
        self.code = 1
        with self.assertRaisesRegex(ValueError, "unbonding"):
            sync.trust_anchor(self.settings)

    def test_pre_upgrade_height(self):
        self.statuses[0]["sync_info"]["latest_block_height"] = "90010"
        with self.assertRaisesRegex(ValueError, "after"):
            sync.trust_anchor(self.settings)

    def test_no_advertised_peers(self):
        for status in self.statuses:
            status["node_info"]["listen_addr"] = "0.0.0.0:26656"
        with self.assertRaisesRegex(ValueError, "snapshot provider"):
            sync.trust_anchor(self.settings)


class ConfigurationTests(unittest.TestCase):
    def test_missing_field_leaves_original_file(self):
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "config/config.toml"
            path.parent.mkdir()
            original = '[statesync]\nenable = false\n[p2p]\npersistent_peers = ""\n'
            path.write_text(original)
            anchor = {"rpc_servers": ["https://one", "https://two"], "trust_height": 100001,
                      "trust_hash": "A" * 64, "trust_period": "24h0m0s", "peers": "peer"}
            with self.assertRaises(ValueError):
                sync.configure(td, anchor)
            self.assertEqual(path.read_text(), original)

    def test_restart_does_not_refresh_completed_state(self):
        with tempfile.TemporaryDirectory() as td:
            home = Path(td)
            (home / "config").mkdir()
            (home / "config/state-sync-complete.json").write_text('{}')
            for name in ("application.db", "blockstore.db"):
                (home / "data" / name).mkdir(parents=True)
            process = MagicMock(returncode=0)
            process.poll.return_value = 0
            with patch.object(sync.socket, "socket"), patch.object(sync, "trust_anchor") as trust, \
                    patch.object(sync.subprocess, "Popen", return_value=process), \
                    patch.object(sync.signal, "signal"):
                self.assertEqual(sync.run_node("fake-node", td, {}), 0)
                trust.assert_not_called()

    def test_duplicate_launcher_rejected(self):
        with tempfile.TemporaryDirectory() as td:
            with (Path(td) / ".congrid-node.lock").open("a") as lock:
                sync.fcntl.flock(lock, sync.fcntl.LOCK_EX | sync.fcntl.LOCK_NB)
                with self.assertRaisesRegex(ValueError, "already running"):
                    sync.run_node("must-not-start", td, {})

    def test_embedded_helper_and_shell_syntax(self):
        installer = ROOT / "cmd/congrid-site/downloads/install.sh"
        text = installer.read_text()
        embedded = text.split("<<'PY_STATE_SYNC'\n", 1)[1].split('\nPY_STATE_SYNC\n', 1)[0]
        self.assertEqual(embedded, (ROOT / "scripts/installer/state_sync.py").read_text().rstrip())
        subprocess.run(["bash", "-n", str(installer)], check=True)


if __name__ == "__main__":
    unittest.main()
