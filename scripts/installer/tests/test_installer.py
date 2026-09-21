"""Execute installer sections in disposable directories, without service changes."""
import hashlib
import io
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
INSTALLER = (ROOT / "cmd/congrid-site/downloads/install.sh").read_text()


class BundleTests(unittest.TestCase):
    def run_bundle(self, bad_checksum=False, source="5907b65faa971afd9e0c29e74284baa03675e37c", override=False):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            work = root / "work"
            work.mkdir()
            archive = root / "congrid-native-linux-amd64.tar.gz"
            files = {
                "bin/content-grid-d": "#!/bin/sh\nprintf 'ibc-transfer-v1\\n'\n",
                "bin/verifierd": "#!/bin/sh\nexit 0\n",
                "bin/indexerd": "#!/bin/sh\nexit 0\n",
                "chromad/server.py": "# fixture\n",
                "chromad/requirements.txt": "# fixture\n",
                "BUILD-INFO": f"source_version=ibc-transfer-v1\nsource_commit={source}\ntarget=linux/amd64\nincludes_pre_upgrade=false\n",
            }
            with tarfile.open(archive, "w:gz") as tar:
                for name, value in files.items():
                    data = value.encode()
                    entry = tarfile.TarInfo("congrid-native/" + name)
                    entry.size = len(data)
                    entry.mode = 0o644
                    tar.addfile(entry, io.BytesIO(data))
            sha = "0" * 64 if bad_checksum else hashlib.sha256(archive.read_bytes()).hexdigest()
            Path(str(archive) + ".sha256").write_text(sha + "  " + archive.name + "\n")
            section = INSTALLER.split('BUNDLE_NAME="${CONGRID_BUNDLE_NAME:', 1)[1]
            section = 'BUNDLE_NAME="${CONGRID_BUNDLE_NAME:' + section.split('# BEGIN EMBEDDED STATE SYNC', 1)[0]
            setup = '''set -euo pipefail
log() { :; }
die() { echo "$*" >&2; exit 1; }
validate_simple_name() { :; }
file_sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
'''
            env = dict(os.environ, HOST_OS="linux", RELEASE_ARCH="amd64", TMP_WORK=str(work),
                       RELEASE_BASE_URL=root.as_uri(), RELEASE_TAG="test-release",
                       DOWNLOAD_BASE_URL="https://must-not-be-used.invalid/network-files",
                       BUNDLE_URL=archive.as_uri() if override else "", BUNDLE_SHA256="",
                       EXPECTED_NODE_VERSION="ibc-transfer-v1",
                       EXPECTED_SOURCE_COMMIT="5907b65faa971afd9e0c29e74284baa03675e37c")
            result = subprocess.run(["bash", "-c", setup + section], env=env, capture_output=True, text=True)
            if result.returncode == 0:
                self.assertFalse((work / "bundle/congrid-native/bin/content-grid-d-pre-upgrade").exists())
            return result

    def test_bundle_without_legacy_uses_release_source(self):
        result = self.run_bundle()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_explicit_bundle_override(self):
        result = self.run_bundle(override=True)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_bad_checksum_rejected(self):
        result = self.run_bundle(bad_checksum=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checksum mismatch", result.stderr)

    def test_wrong_commit_rejected(self):
        result = self.run_bundle(source="wrong-commit")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("source_commit mismatch", result.stderr)


class HomeProtectionTests(unittest.TestCase):
    def check_home(self, components, node_config, owner=False):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            (root / "config").mkdir()
            if node_config:
                (root / "config/config.toml").write_text('moniker="existing"\n')
            if owner:
                (root / "config/congrid-install-owner.json").write_text('{"chain_id":"congrid-main","node_version":"ibc-transfer-v1","bootstrap":"statesync"}')
            section = 'NODE_ALREADY_INITIALIZED=false\n' + INSTALLER.split('NODE_ALREADY_INITIALIZED=false\n', 1)[1].split('# Never overwrite', 1)[0]
            env = dict(os.environ, COMPONENTS_ONLY=components, CONGRID_HOME_DIR=str(root))
            script = 'set -euo pipefail\nrun_root() { "$@"; }\ndie() { echo "$*" >&2; exit 1; }\n'
            before = (root / "config/config.toml").read_bytes() if node_config else None
            result = subprocess.run(["bash", "-c", script + section], env=env, capture_output=True, text=True)
            if node_config:
                self.assertEqual((root / "config/config.toml").read_bytes(), before)
            return result

    def test_unmanaged_node_rejected_without_changes(self):
        result = self.check_home("false", True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("not managed", result.stderr)

    def test_components_cannot_reuse_node_home(self):
        self.assertNotEqual(self.check_home("true", True).returncode, 0)

    def test_components_accept_isolated_home(self):
        self.assertEqual(self.check_home("true", False).returncode, 0)

    def test_managed_home_can_resume(self):
        self.assertEqual(self.check_home("false", True, owner=True).returncode, 0)


if __name__ == "__main__":
    unittest.main()
