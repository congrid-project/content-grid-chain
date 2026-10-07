"""Run the deployment script against disposable files and mocked system services."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "deploy-congrid-site.sh"
PASSWORD = "fixture-cms-password-only"
MOCK_COMMAND = r'''#!/usr/bin/env python3
import hashlib
import os
from pathlib import Path
import shutil
import sys

name = Path(sys.argv[0]).name
args = sys.argv[1:]
root = Path(os.environ["MOCK_DEPLOY_ROOT"])
with (root / "events").open("a") as log:
    log.write(name + " " + " ".join(args) + "\n")

def counter(key):
    path = root / key
    count = int(path.read_text()) + 1 if path.exists() else 1
    path.write_text(str(count))
    return count

if name == "sudo":
    os.execvp(args[3], args[3:])
elif name == "id":
    sys.exit(0)
elif name == "stat":
    print("congridcoin")
elif name == "go":
    if args[0] == "version":
        print("go version go1.25.1 linux/amd64")
    elif args[0] == "build":
        Path(args[args.index("-o") + 1]).write_text("new binary")
elif name == "sha256sum":
    print(hashlib.sha256(Path(args[0]).read_bytes()).hexdigest() + "  " + args[0])
elif name == "cp":
    if args[-1] == str(root / "installed/congrid-site") and os.environ.get("MOCK_ACTIVE_EXECUTABLE") == "1":
        sys.exit("cannot overwrite a running executable")
    shutil.copy2(args[-2], args[-1])
elif name == "mv":
    os.replace(args[-2], args[-1])
elif name == "install":
    target = Path(args[-1])
    if "-d" in args:
        target.mkdir(parents=True, exist_ok=True)
    else:
        shutil.copyfile(args[-2], target)
    target.chmod(int(args[args.index("-m") + 1], 8))
elif name == "chmod":
    Path(args[-1]).chmod(int(args[0], 8))
elif name == "systemctl":
    if args[0] == "--version":
        print("systemd " + os.environ.get("MOCK_SYSTEMD_VERSION", "249"))
    elif args[0] == "show":
        print(str(root / "installed/congrid-site") + os.environ.get("MOCK_EXEC_FLAGS", ""))
    elif args[0] == "daemon-reload":
        if counter("reloads") == 1 and os.environ.get("MOCK_RELOAD_FAIL") == "1":
            sys.exit(1)
    elif args[0] == "restart":
        if counter("restarts") == 1 and os.environ.get("MOCK_RESTART_FAIL") == "1":
            sys.exit(1)
elif name == "curl":
    address = args[-1]
    if "/cms/login" in address:
        print('<form action="/cms/login">')
        if os.environ.get("MOCK_CMS_DISABLED") == "1":
            print("CMS administrator has not been configured.")
    elif "/blog" in address:
        print('<section class="hero blog-hero">')
    else:
        print("publisher=congrid.net wallet=congrid18cepycc5rv3dpe24n0mmdkdqwaruptvkuuurxf")
'''


class DeploymentTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / "repo"
        (self.repo / "scripts").mkdir(parents=True)
        (self.repo / "cmd/congrid-site").mkdir(parents=True)
        (self.repo / "go.mod").write_text("module fixture\n")
        self.bin = self.root / "bin"
        self.bin.mkdir()
        mock = self.bin / "mock-command"
        mock.write_text(MOCK_COMMAND)
        mock.chmod(0o755)
        for name in ("sudo", "systemctl", "journalctl", "curl", "install", "sha256sum",
                     "cp", "mv", "id", "stat", "sleep", "chmod", "go"):
            (self.bin / name).symlink_to(mock.name)
        self.target = self.root / "installed/congrid-site"
        self.target.parent.mkdir()
        self.target.write_text("old binary")
        self.password = self.root / "etc/congrid-site/cms-password"
        self.password.parent.mkdir(parents=True)
        self.password.write_text(PASSWORD + "\n")
        self.drop_in = self.root / "systemd/congrid-site.service.d/50-cms.conf"
        self.env = dict(os.environ, MOCK_DEPLOY_ROOT=str(self.root),
                        PATH=str(self.bin) + os.pathsep + os.environ["PATH"])
        source = SCRIPT.read_text()
        # Keep the full deployment flow while redirecting privileged operations
        # into this fixture and bypassing sudo elevation on the test host.
        source = source.replace('if [[ $EUID -ne 0 ]]; then', 'if false; then')
        source = source.replace('/usr/local/bin/congrid-site', str(self.target))
        source = source.replace('/etc/congrid-site/cms-password', str(self.password))
        source = source.replace('/etc/systemd/system/', str(self.root / 'systemd') + '/')
        source = source.replace('/usr/local/go/bin/go', str(self.bin / 'go'))
        source = source.replace('/home/congridcoin', str(self.root / 'home'))
        source = source.replace('PATH="/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"',
                                'PATH="' + self.env['PATH'] + '"')
        self.script = self.repo / "scripts/deploy-congrid-site.sh"
        self.script.write_text(source)

    def run_deploy(self, **overrides):
        result = subprocess.run(["bash", str(self.script)], env=dict(self.env, **overrides),
                                capture_output=True, text=True, timeout=30)
        self.assertNotIn(PASSWORD, result.stdout + result.stderr)
        return result

    def test_success_configures_cms_without_replacing_exec_start(self):
        result = self.run_deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.target.read_text(), "new binary")
        config = self.drop_in.read_text()
        self.assertIn("StateDirectory=congrid-site/cms", config)
        self.assertIn("LoadCredential=cms-password:" + str(self.password), config)
        self.assertIn("CONGRID_CMS_DB=/var/lib/congrid-site/cms/cms.db", config)
        self.assertIn("CONGRID_CMS_ADMIN_PASSWORD_FILE=/run/credentials/congrid-site.service/cms-password", config)
        self.assertNotIn("ExecStart", config)
        self.assertNotIn(PASSWORD, config)
        self.assertEqual(self.password.stat().st_mode & 0o777, 0o600)
        events = (self.root / "events").read_text()
        self.assertIn("/blog?lang=en", events)
        self.assertIn("/cms/login?lang=en", events)
        self.assertNotIn("--cms-reset-admin-password", events)
        self.assertLess(events.index("systemctl daemon-reload"), events.index("systemctl restart"))

    def test_missing_password_fails_before_install_or_restart(self):
        self.password.unlink()
        result = self.run_deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.target.read_text(), "old binary")
        self.assertFalse(self.drop_in.exists())
        self.assertNotIn("systemctl restart", (self.root / "events").read_text())

    def test_restart_failure_restores_previous_binary_and_drop_in(self):
        self.drop_in.parent.mkdir(parents=True)
        original = "[Service]\nEnvironment=EXISTING=true\n"
        self.drop_in.write_text(original)
        result = self.run_deploy(MOCK_RESTART_FAIL="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.target.read_text(), "old binary")
        self.assertEqual(self.drop_in.read_text(), original)
        self.assertEqual((self.root / "restarts").read_text(), "2")

    def test_unconfigured_login_is_unhealthy_and_removes_new_drop_in(self):
        result = self.run_deploy(MOCK_CMS_DISABLED="1", MOCK_ACTIVE_EXECUTABLE="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.target.read_text(), "old binary")
        self.assertFalse(self.drop_in.exists())
        self.assertEqual((self.root / "restarts").read_text(), "2")
        self.assertIn("rollback completed", result.stdout)

    def test_daemon_reload_failure_restores_configuration_before_binary_swap(self):
        self.drop_in.parent.mkdir(parents=True)
        original = "[Service]\nEnvironment=EXISTING=true\n"
        self.drop_in.write_text(original)
        result = self.run_deploy(MOCK_RELOAD_FAIL="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.target.read_text(), "old binary")
        self.assertEqual(self.drop_in.read_text(), original)

    def test_unsupported_systemd_and_conflicting_flags_do_not_restart(self):
        for setting in ({"MOCK_SYSTEMD_VERSION": "239"},
                        {"MOCK_EXEC_FLAGS": " --cms-db=/old/cms.db"}):
            with self.subTest(setting=setting):
                result = self.run_deploy(**setting)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.target.read_text(), "old binary")
                self.assertFalse(self.drop_in.exists())
                self.assertNotIn("systemctl restart", (self.root / "events").read_text())


if __name__ == "__main__":
    unittest.main()
