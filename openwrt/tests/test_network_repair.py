import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class NetworkRepairScriptTests(unittest.TestCase):
    def run_repair(self, argument, *, running=False, hung=False, status=0):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            calls = root / "calls"
            binary = root / "ackwrap-fixture"
            binary.write_text(
                '#!/bin/sh\nprintf "backend:%s\\n" "$*" >> "$CALLS"\n'
                'echo "fixture result"\nexit "$RESULT"\n',
                encoding="utf-8",
            )
            binary.chmod(0o700)
            script = root / "init.sh"
            script.write_text(
                (ROOT / "openwrt/core/etc/init.d/ackwrap")
                .read_text(encoding="utf-8")
                .replace("/usr/bin/ackwrap", shlex.quote(str(binary))),
                encoding="utf-8",
            )
            wrapper = r'''
stop() { printf 'stop\n' >> "$CALLS"; }
sleep() { :; }
logger() { :; }
config_load() { :; }
config_get() { data_dir="$DATA_DIR"; }
killall() { printf 'kill:%s\n' "$*" >> "$CALLS"; }
pidof() {
  if [ "$1" = ackwrap ]; then [ "$HUNG" = 1 ]; else [ "$RUNNING" = 1 ]; fi
}
. "$INIT"
network_repair "$@"
'''
            env = dict(os.environ, CALLS=str(calls), INIT=str(script), DATA_DIR=str(root),
                       RESULT=str(status), RUNNING=str(int(running)), HUNG=str(int(hung)))
            args = ["sh", "-c", wrapper, "fixture"]
            if argument:
                args.append(argument)
            result = subprocess.run(args, env=env, capture_output=True, text=True, timeout=5)
            recorded = calls.read_text() if calls.exists() else ""
            return result, recorded

    def test_luci_force_stops_service_and_passes_force(self):
        result, calls = self.run_repair("--force", running=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, "stop\nbackend:network-repair --force\n")
        self.assertEqual(result.stdout, "OK\nfixture result\n")

    def test_force_terminates_hung_backend(self):
        result, calls = self.run_repair("--force", running=True, hung=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, "stop\nkill:-KILL ackwrap\nbackend:network-repair --force\n")

    def test_command_failure_reaches_luci(self):
        result, calls = self.run_repair("--force", status=1)
        self.assertEqual(result.returncode, 1)
        self.assertIn("backend:network-repair --force", calls)
        self.assertEqual(result.stdout, "ERROR\nfixture result\n")

    def test_non_force_still_protects_running_core(self):
        result, calls = self.run_repair("", running=True)
        self.assertEqual(result.returncode, 2)
        self.assertEqual(calls, "")
        self.assertTrue(result.stdout.startswith("ERROR\n"))

    def test_unknown_argument_does_not_stop_services(self):
        result, calls = self.run_repair("--unknown")
        self.assertEqual(result.returncode, 1)
        self.assertEqual(calls, "")


if __name__ == "__main__":
    unittest.main()
