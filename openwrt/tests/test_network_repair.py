import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class NetworkRepairScriptTests(unittest.TestCase):
    def test_linux_shebang_script_shares_name_but_not_daemon_executable(self):
        with tempfile.TemporaryDirectory() as directory:
            script = Path(directory) / "ackwrap"
            script.write_text("#!/bin/sh\ncat /proc/$$/comm\nreadlink /proc/$$/exe\n", encoding="utf-8")
            script.chmod(0o700)
            result = subprocess.run([str(script)], capture_output=True, text=True, timeout=5)
            self.assertEqual(result.returncode, 0, result.stderr)
            name, executable = result.stdout.splitlines()
            self.assertEqual(name, "ackwrap")
            self.assertNotEqual(Path(executable).name, "ackwrap")

    def run_repair(self, argument, *, running=False, hung=False, status=0,
                   action="network_repair", enabled=True, start_fails=False,
                   never_running=False):
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
stop() { printf 'stop\n' >> "$CALLS"; DAEMON_RUNNING=0; }
start() {
  printf 'start\n' >> "$CALLS"
  [ "$START_FAILS" = 1 ] && return 1
  if [ "$ENABLED" = 1 ] && [ "$NEVER_RUNNING" = 0 ]; then DAEMON_RUNNING=1; fi
  return 0
}
sleep() { :; }
logger() { :; }
config_load() { :; }
config_get() { data_dir="$DATA_DIR"; }
config_get_bool() { enabled="$ENABLED"; }
killall() {
  # BusyBox matches the init script's comm too. A basename kill kills this caller.
  if [ "$2" = ackwrap ]; then exit 137; fi
  [ "$2" = "$BINARY" ] || exit 99
  printf 'kill:%s daemon\n' "$1" >> "$CALLS"
}
pidof() {
  case "$1" in
    ackwrap) printf '%s\n' "$$"; return 0 ;;
    "$BINARY")
      if [ "$ACTION" = apply_settings ]; then
        [ "${DAEMON_RUNNING:-0}" = 1 ]
      else
        [ "$HUNG" = 1 ]
      fi
      ;;
    sing-box) [ "$RUNNING" = 1 ] ;;
    *) return 1 ;;
  esac
}
. "$INIT"
"$ACTION" "$@"
'''
            env = dict(os.environ, CALLS=str(calls), INIT=str(script), DATA_DIR=str(root),
                       BINARY=str(binary), ACTION=action, ENABLED=str(int(enabled)),
                       START_FAILS=str(int(start_fails)), NEVER_RUNNING=str(int(never_running)),
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
        self.assertEqual(calls, "stop\nkill:-KILL daemon\nbackend:network-repair --force\n")

    def test_force_ignores_same_named_init_script_and_returns_result(self):
        result, calls = self.run_repair("--force")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, "stop\nbackend:network-repair --force\n")
        self.assertEqual(result.stdout, "OK\nfixture result\n")

    def test_apply_starts_enabled_service_even_without_config_changes(self):
        result, calls = self.run_repair("", action="apply_settings")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, "stop\nstart\n")
        self.assertTrue(result.stdout.startswith("OK\n"))
        self.assertIn("已启动", result.stdout)

    def test_apply_stops_disabled_service(self):
        result, calls = self.run_repair("", action="apply_settings", enabled=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, "stop\nstart\n")
        self.assertIn("已停止", result.stdout)

    def test_apply_reports_invalid_start_configuration(self):
        result, _ = self.run_repair("", action="apply_settings", start_fails=True)
        self.assertEqual(result.returncode, 1)
        self.assertTrue(result.stdout.startswith("ERROR\n"))

    def test_apply_does_not_report_success_when_daemon_fails_to_start(self):
        result, _ = self.run_repair("", action="apply_settings", never_running=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn("未达到预期运行状态", result.stdout)

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
