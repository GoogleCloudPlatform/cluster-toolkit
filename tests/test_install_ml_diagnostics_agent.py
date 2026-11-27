# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Unit tests for modules/scripts/startup-script/files/install_ml_diagnostics_agent.sh."""

import os
from pathlib import Path
import shlex
import stat
import subprocess
import tempfile
import unittest

REPO_ROOT = Path(__file__).resolve().parents[1]
SCRIPT_PATH = (
    REPO_ROOT
    / "modules/scripts/startup-script/files/install_ml_diagnostics_agent.sh"
)


def _write_executable(path: Path, content: str) -> None:
    path.write_text(content, encoding="utf-8")
    path.chmod(0o755)


class InstallMlDiagnosticsAgentTests(unittest.TestCase):
    """Tests CLI parsing, APT discovery, rollback/commit, and mocked flows."""

    def setUp(self) -> None:
        self._tmpdir = tempfile.TemporaryDirectory()
        self.tmp = Path(self._tmpdir.name)
        self.root = self.tmp / "root"
        (self.root / "etc/apt/sources.list.d").mkdir(parents=True)
        (self.root / "usr/share/keyrings").mkdir(parents=True)
        (self.root / "etc/systemd/system").mkdir(parents=True)
        (self.root / "usr/bin").mkdir(parents=True)
        (self.root / "etc/apt/sources.list").write_text("", encoding="utf-8")
        self.mock_bin = self.tmp / "mock-bin"
        self.mock_bin.mkdir()
        self.state = self.tmp / "state"
        self.state.mkdir()
        self.log_file = self.tmp / "calls.log"
        self.log_file.write_text("", encoding="utf-8")
        self._install_default_mocks()

    def tearDown(self) -> None:
        self._tmpdir.cleanup()

    def _install_default_mocks(self) -> None:
        _write_executable(
            self.mock_bin / "uname",
            "#!/bin/bash\n"
            'if [[ -f "${MOCK_STATE}/arch" ]]; then cat "${MOCK_STATE}/arch"; else echo x86_64; fi\n',
        )
        _write_executable(
            self.mock_bin / "logger",
            '#!/bin/bash\necho "logger $*" >>"${MOCK_LOG}"\n',
        )
        _write_executable(
            self.mock_bin / "dpkg",
            '#!/bin/bash\necho "dpkg $*" >>"${MOCK_LOG}"\n',
        )
        _write_executable(
            self.mock_bin / "dpkg-query",
            """#!/bin/bash
pkg=""
for arg in "$@"; do
  pkg="${arg}"
done
if [[ -f "${MOCK_STATE}/pkg_${pkg}" ]]; then
  status="$(cat "${MOCK_STATE}/pkg_${pkg}")"
  ver="$(cat "${MOCK_STATE}/ver_${pkg}" 2>/dev/null || echo "1.0.0")"
  if [[ "$*" == *'${Version}'* ]]; then
    printf '%s\\t%s\\n' "${status}" "${ver}"
  else
    printf '%s\\n' "${status}"
  fi
  exit 0
fi
exit 1
""",
        )
        _write_executable(
            self.mock_bin / "apt-mark",
            """#!/bin/bash
echo "apt-mark $*" >>"${MOCK_LOG}"
args=()
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then shift 2; continue; fi
  args+=("$1"); shift
done
cmd="${args[0]}"
pkg="${args[1]:-}"
case "${cmd}" in
showhold)
  if [[ -f "${MOCK_STATE}/pkg_${pkg}" ]] && [[ "$(cat "${MOCK_STATE}/pkg_${pkg}")" == "hold ok installed" ]]; then
    echo "${pkg}"
  fi
  ;;
hold)
  if [[ -f "${MOCK_STATE}/fail_hold" ]]; then
    echo "E: simulated hold error" >&2
    exit 1
  fi
  echo "hold ok installed" >"${MOCK_STATE}/pkg_${pkg}"
  ;;
unhold)
  if [[ -f "${MOCK_STATE}/fail_unhold" ]]; then
    echo "E: simulated unhold error" >&2
    exit 1
  fi
  if [[ -f "${MOCK_STATE}/pkg_${pkg}" ]]; then
    echo "install ok installed" >"${MOCK_STATE}/pkg_${pkg}"
  fi
  ;;
esac
""",
        )
        _write_executable(
            self.mock_bin / "apt-cache",
            """#!/bin/bash
echo "apt-cache $*" >>"${MOCK_LOG}"
cmd="$1"
pkg="$2"
case "${cmd}" in
policy)
  if [[ -f "${MOCK_STATE}/cand_${pkg}" ]]; then
    echo "${pkg}:"
    echo "  Installed: (none)"
    echo "  Candidate: $(cat "${MOCK_STATE}/cand_${pkg}")"
  else
    echo "N: Unable to locate package ${pkg}"
  fi
  ;;
madison)
  if [[ -f "${MOCK_STATE}/madison_${pkg}" ]]; then
    cat "${MOCK_STATE}/madison_${pkg}"
  fi
  ;;
esac
""",
        )
        _write_executable(
            self.mock_bin / "curl",
            """#!/bin/bash
echo "curl $*" >>"${MOCK_LOG}"
if [[ -f "${MOCK_STATE}/fail_curl" ]]; then
  echo "curl: simulated network error" >&2
  exit 22
fi
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--output" ]]; then out="$2"; shift 2; continue; fi
  shift
done
echo "MOCK-ARMORED-KEY" >"${out}"
""",
        )
        _write_executable(
            self.mock_bin / "gpg",
            """#!/bin/bash
echo "gpg $*" >>"${MOCK_LOG}"
if [[ -f "${MOCK_STATE}/fail_gpg" ]]; then
  echo "gpg: simulated dearmor failure" >&2
  exit 2
fi
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--output" ]]; then out="$2"; shift 2; continue; fi
  shift
done
echo "MOCK-DEARMORED-KEY" >"${out}"
""",
        )
        _write_executable(
            self.mock_bin / "apt-get",
            """#!/bin/bash
echo "apt-get $*" >>"${MOCK_LOG}"
if [[ "$*" == *" update"* ]]; then
  if [[ -f "${MOCK_STATE}/fail_apt_update" ]]; then
    echo "E: simulated apt update failure" >&2
    exit 100
  fi
  if [[ -f "${MOCK_STATE}/fail_scoped_update" ]] && [[ "$*" == *"Dir::Etc::sourcelist="* ]]; then
    echo "E: simulated scoped update failure" >&2
    exit 100
  fi
  if [[ "$*" == *"ar-transport"* || "$*" == *"a4high-ar"* || "$*" != *"Dir::Etc::sourcelist="* ]]; then
    echo "20240529-1" >"${MOCK_STATE}/cand_apt-transport-artifact-registry"
  fi
  if [[ "$*" == *"diagon"* || "$*" == *"osconfig"* || "$*" == *"sourceparts."* || "$*" != *"Dir::Etc::sourcelist="* ]]; then
    echo "1.2.3-1" >"${MOCK_STATE}/cand_mldiagnostics-daemon"
    printf ' mldiagnostics-daemon | 1.2.3-1 | ar+https://example daemon-apt/main amd64 Packages\\n' >"${MOCK_STATE}/madison_mldiagnostics-daemon"
  fi
  exit 0
fi
if [[ "$*" == *" install "* ]]; then
  if [[ -f "${MOCK_STATE}/sigterm_on_install" ]]; then
    kill -TERM "${PPID}"
    exit 143
  fi
  if [[ -f "${MOCK_STATE}/fail_apt_install" ]]; then
    echo "E: simulated apt install failure" >&2
    exit 100
  fi
  pkg_arg=""
  for arg in "$@"; do
    pkg_arg="${arg}"
  done
  pkg="${pkg_arg%%=*}"
  ver="${pkg_arg#*=}"
  [[ "${ver}" == "${pkg}" ]] && ver="1.2.3-1"
  echo "install ok installed" >"${MOCK_STATE}/pkg_${pkg}"
  echo "${ver}" >"${MOCK_STATE}/ver_${pkg}"
  if [[ "${pkg}" == "mldiagnostics-daemon" ]]; then
    printf '#!/bin/bash\\nexit 0\\n' >"${MOCK_ROOT}/usr/bin/mldiagnostics_extension"
    chmod +x "${MOCK_ROOT}/usr/bin/mldiagnostics_extension"
  fi
  exit 0
fi
""",
        )
        _write_executable(
            self.mock_bin / "systemctl",
            """#!/bin/bash
echo "systemctl $*" >>"${MOCK_LOG}"
case "$1" in
daemon-reload)
  [[ -f "${MOCK_STATE}/fail_systemctl" ]] && exit 1
  exit 0
  ;;
is-enabled)
  [[ -f "${MOCK_STATE}/service_enabled" ]] && exit 0
  exit 1
  ;;
enable)
  [[ -f "${MOCK_STATE}/fail_systemctl" ]] && exit 1
  touch "${MOCK_STATE}/service_enabled"
  exit 0
  ;;
restart|start)
  [[ -f "${MOCK_STATE}/fail_systemctl" ]] && exit 1
  touch "${MOCK_STATE}/service_active"
  exit 0
  ;;
is-active)
  [[ -f "${MOCK_STATE}/service_active" ]] && exit 0
  exit 3
  ;;
esac
""",
        )
        (self.state / "pkg_ca-certificates").write_text(
            "install ok installed\n", encoding="utf-8"
        )

    def _run_sourced(
        self,
        body: str,
        *,
        mock_root: bool = True,
        override_root: bool = True,
        extra_env: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess:
        env = os.environ.copy()
        for var in (
            "BASH_ENV",
            "ENV",
            "CDPATH",
            "SHELLOPTS",
            "BASHOPTS",
            "GREP_OPTIONS",
            "POSIXLY_CORRECT",
            "DIAGON_APT_REPO_URL",
            "DIAGON_APT_SUITE",
            "DIAGON_APT_COMPONENT",
            "DIAGON_GPG_KEY_URL",
            "DIAGON_CHANNEL",
        ):
            env.pop(var, None)
        if extra_env:
            env.update(extra_env)
        env["LC_ALL"] = "C"
        env["PATH"] = f"{self.mock_bin}:{env.get('PATH', '')}"
        env["MOCK_LOG"] = str(self.log_file)
        env["MOCK_STATE"] = str(self.state)
        env["MOCK_ROOT"] = str(self.root)

        preamble = [f"source {shlex.quote(str(SCRIPT_PATH))}"]
        if mock_root:
            preamble.extend(
                [
                    f"APT_SOURCES_LIST={shlex.quote(str(self.root / 'etc/apt/sources.list'))}",
                    f"APT_SOURCES_DIR={shlex.quote(str(self.root / 'etc/apt/sources.list.d'))}",
                    f"AR_TRANSPORT_KEYRING={shlex.quote(str(self.root / 'usr/share/keyrings/google-cloud-ar-transport.gpg'))}",
                    f"AR_TRANSPORT_LIST={shlex.quote(str(self.root / 'etc/apt/sources.list.d/google-cloud-ar-transport.list'))}",
                    f"DIAGON_KEYRING={shlex.quote(str(self.root / 'usr/share/keyrings/diagon-daemon.gpg'))}",
                    f"DIAGON_LIST={shlex.quote(str(self.root / 'etc/apt/sources.list.d/diagon-daemon.list'))}",
                    f"BINARY_PATH={shlex.quote(str(self.root / 'usr/bin/mldiagnostics_extension'))}",
                    f"SERVICE_FILE={shlex.quote(str(self.root / 'etc/systemd/system/diagon-daemon.service'))}",
                ]
            )
        if override_root:
            preamble.append("is_root() { return 0; }")

        script = "\n".join(preamble) + "\n" + body
        return subprocess.run(
            ["bash", "-c", script],
            env=env,
            capture_output=True,
            text=True,
            check=False,
            timeout=30,
        )

    # --- CLI Parsing & Validation ---

    def test_parse_args_and_env_overrides(self) -> None:
        res = self._run_sourced(
            'parse_args "" --version=1.0.1\n'
            'printf "%s|%s|%s|%s\\n" "${VERSION}" "${APT_SUITE}" '
            '"${APT_COMPONENT}" "${CHANNEL}"\n',
            extra_env={
                "DIAGON_APT_SUITE": "custom-suite",
                "DIAGON_APT_COMPONENT": "contrib",
                "DIAGON_CHANNEL": "custom.googleapis.com/v2",
            },
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertEqual(
            res.stdout.strip(),
            "1.0.1|custom-suite|contrib|custom.googleapis.com/v2",
        )

        res_split = self._run_sourced(
            'parse_args --version 1:2.3.4~rc1+git-1\nprintf "%s\\n" "${VERSION}"\n'
        )
        self.assertEqual(res_split.returncode, 0, res_split.stderr)
        self.assertEqual(res_split.stdout.strip(), "1:2.3.4~rc1+git-1")

    def test_help_prints_usage_and_exits_zero(self) -> None:
        res = self._run_sourced("main --help\n")
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertIn("--version=VERSION", res.stdout)
        self.assertIn("diagon-daemon.service", res.stdout)
        self.assertIn("Fail-open", res.stdout)

    def test_parse_args_and_config_reject_invalid_values(self) -> None:
        invalid_cli_cases = [
            ["--unknown-flag"],
            ["--apt-repo-url=https://example.com/repo"],
            ["--version"],
            ["--version=v1.0.1"],
            ["--version=1.0;id"],
        ]
        for args in invalid_cli_cases:
            quoted = " ".join(shlex.quote(a) for a in args)
            with self.subTest(args=args):
                res = self._run_sourced(f"parse_args {quoted}\n")
                self.assertNotEqual(res.returncode, 0, f"expected reject for {args}")
                self.assertIn("WARNING:", res.stderr)

        invalid_env_cases = [
            {"DIAGON_CHANNEL": "foo%nbar"},
            {"DIAGON_CHANNEL": "foo$bar"},
            {"DIAGON_CHANNEL": "foo\\bar"},
            {"DIAGON_CHANNEL": 'foo"bar'},
            {"DIAGON_APT_SUITE": "daemon#apt"},
            {"DIAGON_APT_COMPONENT": "main[1]"},
            {"DIAGON_APT_REPO_URL": "https://example.com\\bad"},
            {"DIAGON_APT_REPO_URL": "https://example.com#bad"},
            {"DIAGON_GPG_KEY_URL": "http://insecure.example/key.gpg"},
            {"DIAGON_GPG_KEY_URL": "file:///etc/passwd"},
            {"DIAGON_GPG_KEY_URL": "https://"},
        ]
        for extra_env in invalid_env_cases:
            with self.subTest(env=extra_env):
                res = self._run_sourced("validate_config\n", extra_env=extra_env)
                self.assertNotEqual(
                    res.returncode, 0, f"expected reject for {extra_env}"
                )
                self.assertIn("WARNING:", res.stderr)

    # --- APT Discovery ---

    def test_apt_list_and_sources_discovery(self) -> None:
        sources_d = self.root / "etc/apt/sources.list.d"
        (sources_d / "active.list").write_text(
            "# comment\n"
            "deb-src ar+https://example.com/repo daemon-apt main\n"
            "deb [signed-by=/usr/share/keyrings/x.gpg] ar+https://example.com/repo/ daemon-apt main # inline\n",
            encoding="utf-8",
        )
        (sources_d / "ignored.list.save").write_text(
            "deb ar+https://example.com/repo daemon-apt main\n", encoding="utf-8"
        )
        (sources_d / "ignored.list.disabled").write_text(
            "deb ar+https://example.com/repo daemon-apt main\n", encoding="utf-8"
        )
        (sources_d / ".hidden.list").write_text(
            "deb ar+https://example.com/repo daemon-apt main\n", encoding="utf-8"
        )
        (sources_d / "disabled.sources").write_text(
            "Types: deb\n"
            "Enabled: no\n"
            "URIs: ar+https://example.com/repo\n"
            "Suites: daemon-apt\n"
            "Components: main\n",
            encoding="utf-8",
        )
        (sources_d / "active.sources").write_text(
            "Types: deb\n"
            "Enabled: false\n"
            "URIs: ar+https://other.example/repo\n"
            "Suites: other\n"
            "Components: main\n\n"
            "Types: deb deb-src\n"
            "URIs:\n"
            "  ar+https://example.com/repo/\n"
            "Suites: daemon-apt\n"
            "Components: main\n",
            encoding="utf-8",
        )

        res = self._run_sourced(
            'find_existing_apt_sources "ar+https://example.com/repo" "daemon-apt" ""\n'
            'printf "%s\\n" ${MATCHED_APT_SOURCES[@]+"${MATCHED_APT_SOURCES[@]}"}\n'
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        matched = sorted(Path(p).name for p in res.stdout.splitlines() if p)
        self.assertEqual(matched, ["active.list", "active.sources"])

    # --- File Tracking, Rollback, Commit, and Permissions ---

    def test_tracked_file_backup_rollback_commit_and_chmod(self) -> None:
        existing = self.root / "etc/apt/sources.list.d/diagon-daemon.list"
        existing.write_text("original-content\n", encoding="utf-8")
        existing.chmod(0o600)
        new_file = self.root / "usr/share/keyrings/diagon-daemon.gpg"

        # 1. Identical content with drifted 0600 mode gets corrected to 0644.
        src_same = self.tmp / "same.list"
        src_same.write_text("original-content\n", encoding="utf-8")
        res = self._run_sourced(
            f"install_tracked_file {shlex.quote(str(src_same))} {shlex.quote(str(existing))} 0644\n"
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertEqual(stat.S_IMODE(existing.stat().st_mode), 0o644)

        # 2. Multiple overwrites preserve first backup; rollback restores original & deletes new file.
        src_v1 = self.tmp / "v1.list"
        src_v2 = self.tmp / "v2.list"
        src_key = self.tmp / "k.gpg"
        src_v1.write_text("modified-v1\n", encoding="utf-8")
        src_v2.write_text("modified-v2\n", encoding="utf-8")
        src_key.write_text("key-bytes\n", encoding="utf-8")
        res = self._run_sourced(
            f"install_tracked_file {shlex.quote(str(src_v1))} {shlex.quote(str(existing))} 0644\n"
            f"install_tracked_file {shlex.quote(str(src_v2))} {shlex.quote(str(existing))} 0644\n"
            f"install_tracked_file {shlex.quote(str(src_key))} {shlex.quote(str(new_file))} 0644\n"
            "rollback_apt_files\n"
            'rm -rf -- "${TMP_DIR}"\n'
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertEqual(existing.read_text(encoding="utf-8"), "original-content\n")
        self.assertFalse(new_file.exists())

        # 3. commit_apt_changes prevents subsequent rollback_apt_files from reverting committed files.
        res = self._run_sourced(
            f"install_tracked_file {shlex.quote(str(src_v1))} {shlex.quote(str(existing))} 0644\n"
            f"install_tracked_file {shlex.quote(str(src_key))} {shlex.quote(str(new_file))} 0644\n"
            "commit_apt_changes\n"
            "rollback_apt_files\n"
            'rm -rf -- "${TMP_DIR}"\n'
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertEqual(existing.read_text(encoding="utf-8"), "modified-v1\n")
        self.assertTrue(new_file.exists())

    def test_madison_ignores_deb_src_sources_entries(self) -> None:
        (self.state / "madison_mldiagnostics-daemon").write_text(
            " mldiagnostics-daemon | 2.0.0-1 | https://example daemon-apt/main Sources\n"
            " mldiagnostics-daemon | 1.2.3-1 | https://example daemon-apt/main amd64 Packages\n",
            encoding="utf-8",
        )
        res_src_only = self._run_sourced(
            'VERSION="2.0.0-1"; is_pkg_version_available\n'
        )
        self.assertNotEqual(res_src_only.returncode, 0)

        res_pkg = self._run_sourced(
            'VERSION="1.2.3-1"; is_pkg_version_available\n'
        )
        self.assertEqual(res_pkg.returncode, 0)

    # --- End-to-End Mocked Flows ---

    def test_fail_open_on_non_x86_64_and_non_root(self) -> None:
        (self.state / "arch").write_text("aarch64\n", encoding="utf-8")
        res = self._run_sourced("main\n")
        self.assertEqual(res.returncode, 0)
        self.assertIn("Unsupported architecture 'aarch64'", res.stderr)
        self.assertNotIn("apt-get", self.log_file.read_text(encoding="utf-8"))

        (self.state / "arch").unlink()
        res_non_root = self._run_sourced(
            "is_root() { return 1; }\nmain\n", override_root=False
        )
        self.assertEqual(res_non_root.returncode, 0)
        self.assertIn("Root privileges are required", res_non_root.stderr)

    def test_fresh_install_hardened_unit_and_bounded_timeouts(self) -> None:
        res = self._run_sourced("main\n")
        self.assertEqual(res.returncode, 0, res.stderr)
        unit = (
            self.root / "etc/systemd/system/diagon-daemon.service"
        ).read_text(encoding="utf-8")
        self.assertIn("StartLimitIntervalSec=0", unit)
        self.assertNotIn("StartLimitBurst=", unit)
        self.assertIn("Restart=on-failure", unit)
        self.assertIn("RestartSec=15", unit)
        self.assertIn("OOMScoreAdjust=500", unit)
        calls = self.log_file.read_text(encoding="utf-8")
        self.assertIn("DPkg::Lock::Timeout=30", calls)
        self.assertIn("Acquire::http::Timeout=15", calls)
        self.assertIn("Acquire::https::Timeout=15", calls)
        self.assertIn("Acquire::Retries=1", calls)
        self.assertIn("--proto =https", calls)
        self.assertIn("--tlsv1.2", calls)

    def test_broken_url_rolls_back_preexisting_apt_files(self) -> None:
        (self.state / "pkg_apt-transport-artifact-registry").write_text(
            "install ok installed\n", encoding="utf-8"
        )
        diagon_list = self.root / "etc/apt/sources.list.d/diagon-daemon.list"
        diagon_gpg = self.root / "usr/share/keyrings/diagon-daemon.gpg"
        diagon_list.write_text("deb ar+https://orig daemon-apt main\n", encoding="utf-8")
        diagon_gpg.write_text("ORIG-KEY\n", encoding="utf-8")
        (self.state / "fail_apt_update").write_text("1\n", encoding="utf-8")

        res = self._run_sourced(
            "main\n",
            extra_env={"DIAGON_APT_REPO_URL": "ar+https://broken.example/bad"},
        )
        self.assertEqual(res.returncode, 0)
        self.assertIn("WARNING:", res.stderr)
        self.assertEqual(
            diagon_list.read_text(encoding="utf-8"),
            "deb ar+https://orig daemon-apt main\n",
        )
        self.assertEqual(diagon_gpg.read_text(encoding="utf-8"), "ORIG-KEY\n")

    def test_post_install_commit_preserves_apt_files_when_systemctl_fails(self) -> None:
        (self.state / "pkg_apt-transport-artifact-registry").write_text(
            "install ok installed\n", encoding="utf-8"
        )
        (self.state / "fail_systemctl").write_text("1\n", encoding="utf-8")

        res = self._run_sourced("main\n")
        self.assertEqual(res.returncode, 0)
        self.assertIn("WARNING:", res.stderr)
        # APT files committed after apt-get install must NOT be rolled back!
        self.assertTrue(
            (self.root / "etc/apt/sources.list.d/diagon-daemon.list").exists()
        )
        self.assertTrue(
            (self.root / "usr/share/keyrings/diagon-daemon.gpg").exists()
        )

    def test_hold_failure_warns_and_still_starts_service(self) -> None:
        (self.state / "pkg_apt-transport-artifact-registry").write_text(
            "install ok installed\n", encoding="utf-8"
        )
        (self.state / "fail_hold").write_text("1\n", encoding="utf-8")

        res = self._run_sourced("main --version=1.2.3-1\n")
        self.assertEqual(res.returncode, 0)
        self.assertIn("Failed to hold mldiagnostics-daemon", res.stderr)
        self.assertTrue((self.state / "service_active").exists())

    def test_restore_hold_on_failed_install_and_sigterm(self) -> None:
        for mode in ("fail_install", "sigterm"):
            with self.subTest(mode=mode):
                (self.state / "pkg_apt-transport-artifact-registry").write_text(
                    "install ok installed\n", encoding="utf-8"
                )
                (self.state / "pkg_mldiagnostics-daemon").write_text(
                    "hold ok installed\n", encoding="utf-8"
                )
                (self.state / "ver_mldiagnostics-daemon").write_text(
                    "1.0.0-1\n", encoding="utf-8"
                )
                for flag in ("fail_apt_install", "sigterm_on_install"):
                    (self.state / flag).unlink(missing_ok=True)
                if mode == "fail_install":
                    (self.state / "fail_apt_install").write_text("1\n", encoding="utf-8")
                else:
                    (self.state / "sigterm_on_install").write_text("1\n", encoding="utf-8")

                res = self._run_sourced("main --version=1.2.3-1\n")
                self.assertEqual(res.returncode, 0)
                self.assertIn("WARNING:", res.stderr)
                if mode == "sigterm":
                    self.assertIn("Interrupted by signal", res.stderr)
                self.assertEqual(
                    (self.state / "pkg_mldiagnostics-daemon")
                    .read_text(encoding="utf-8")
                    .strip(),
                    "hold ok installed",
                )

    def test_scoped_sources_refresh_and_no_full_update_on_scoped_list_fail(self) -> None:
        (self.state / "pkg_apt-transport-artifact-registry").write_text(
            "install ok installed\n", encoding="utf-8"
        )
        # Active DEB822 .sources file uses scoped Dir::Etc::sourceparts update.
        (self.root / "etc/apt/sources.list.d/managed.sources").write_text(
            "Types: deb\n"
            "URIs: ar+https://us-central1-apt.pkg.dev/projects/hypercomputediagon-release\n"
            "Suites: daemon-apt\n"
            "Components: main\n",
            encoding="utf-8",
        )
        res = self._run_sourced("main\n")
        self.assertEqual(res.returncode, 0, res.stderr)
        calls = self.log_file.read_text(encoding="utf-8")
        self.assertIn("Dir::Etc::sourceparts=", calls)
        self.assertFalse(
            (self.root / "etc/apt/sources.list.d/diagon-daemon.list").exists()
        )

        # Failed scoped .list refresh must NOT fall back to full apt-get update.
        (self.root / "etc/apt/sources.list.d/managed.sources").unlink()
        (self.state / "pkg_mldiagnostics-daemon").unlink(missing_ok=True)
        (self.state / "cand_mldiagnostics-daemon").unlink(missing_ok=True)
        self.log_file.write_text("", encoding="utf-8")
        (self.root / "etc/apt/sources.list.d/osconfig_diagon.list").write_text(
            "deb ar+https://us-central1-apt.pkg.dev/projects/hypercomputediagon-release daemon-apt main\n",
            encoding="utf-8",
        )
        (self.state / "fail_scoped_update").write_text("1\n", encoding="utf-8")
        res2 = self._run_sourced("main\n")
        self.assertEqual(res2.returncode, 0)
        calls2 = self.log_file.read_text(encoding="utf-8")
        for line in calls2.splitlines():
            if line.startswith("apt-get ") and line.endswith(" update"):
                self.assertIn("Dir::Etc::sourcelist=", line)

    def test_wall_clock_deadline_skips_full_update_retry(self) -> None:
        (self.state / "pkg_apt-transport-artifact-registry").write_text(
            "install ok installed\n", encoding="utf-8"
        )
        (self.state / "fail_apt_install").write_text("1\n", encoding="utf-8")
        res = self._run_sourced("SECONDS=125\nmain\n")
        self.assertEqual(res.returncode, 0)
        self.assertIn("skipping full APT cache refresh retry", res.stderr)
        calls = self.log_file.read_text(encoding="utf-8")
        for line in calls.splitlines():
            if line.startswith("apt-get ") and line.endswith(" update"):
                self.assertIn("Dir::Etc::sourcelist=", line)

    def test_https_repo_skips_ar_transport(self) -> None:
        res = self._run_sourced(
            "main\n",
            extra_env={
                "DIAGON_APT_REPO_URL": "https://mirror.internal.example/diagon"
            },
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        self.assertFalse(
            (
                self.root / "etc/apt/sources.list.d/google-cloud-ar-transport.list"
            ).exists()
        )
        self.assertNotIn(
            "packages.cloud.google.com", self.log_file.read_text(encoding="utf-8")
        )


if __name__ == "__main__":
    unittest.main()
