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

"""How the web server's startup script reads its configuration.

The script reads the configuration (which holds the admin password) from a
Secret Manager secret, never from the control bucket, and disables the version
it read only once the server is installed and running, so a first boot that
stops earlier leaves it enabled for the next boot to read; an installed server
disables any version still enabled each time it starts. The checks read the
script's text, and run the parts that read the configuration and disable
versions against fake curl, gcloud and yq commands, with the configuration
file's path moved into a scratch directory, so nothing reaches Google Cloud.
"""

import os
import shutil
import subprocess
import tempfile
from pathlib import Path

from django.test import SimpleTestCase

OFE_DIR = Path(__file__).resolve().parents[3]
STARTUP = OFE_DIR / "infrastructure_files/gcs_bucket/webserver/startup.sh"

PROJECT = "test-project"
SECRET = "testdep-server-config"
VERSION = "7"
CONFIG_PATH = "/tmp/config"
YQ = "/usr/local/bin/yq"
# Where the script keeps the TKFE code and its first-boot markers; the run
# tests set these variables to scratch paths instead.
PATH_LINES = (
    "tkfe_code_dir=/opt/gcluster/cluster-toolkit\n"
    "ofe_state_dir=/var/lib/ofe-server\n"
    "first_boot_started=${ofe_state_dir}/first-boot-started\n"
    "first_boot_fetched=${ofe_state_dir}/first-boot-fetched\n"
    "first_boot_complete=${ofe_state_dir}/first-boot-complete\n"
)
INSTALLED_CHECK = "\nif [[ -d ${tkfe_code_dir} ]]; then\n"
BOOTSTRAP_PY = (
    "/opt/gcluster/cluster-toolkit/community/front-end/ofe/website/ghpcfe/"
    "management/commands/ofe_bootstrap_credentials.py"
)

# Answers the metadata key ofe-server-config-secret with FAKE_SECRET (curl
# --fail gives no output for a missing key), and the application's port as
# FAKE_APP_RCS says.
CURL_FAKE = r"""#!/bin/bash
if [[ $* == *http://127.0.0.1:8001/* ]]; then
	# The application's port: call <n> exits with the n-th number in
	# FAKE_APP_RCS (the last repeats; default 0, answered)
	(IFS=$'\x1f'; printf 'curl\x1f%s\n' "$*") >>"${FAKE_LOG}"
	n=$(grep -c '^curl' "${FAKE_LOG}")
	read -r -a rcs <<<"${FAKE_APP_RCS:-0}"
	exit "${rcs[n - 1]:-${rcs[-1]}}"
fi
if [[ $* == *attributes/ofe-server-config-secret* && -n ${FAKE_SECRET:-} ]]; then
	printf '%s' "${FAKE_SECRET}"
	exit 0
fi
exit 22
"""

# Logs each call. describe answers FAKE_VERSION (default 7), list answers
# FAKE_ENABLED, and disable fails for the version named in FAKE_DISABLE_FAIL
# and kills the calling script for the version named in FAKE_DISABLE_KILL.
GCLOUD_FAKE = r"""#!/bin/bash
(IFS=$'\x1f'; printf 'gcloud\x1f%s\n' "$*") >>"${FAKE_LOG}"
case "$1 $2 $3" in
"secrets versions describe")
	[[ ${FAKE_DESCRIBE_RC:-0} -eq 0 ]] || exit "${FAKE_DESCRIBE_RC}"
	printf '%s\n' "${FAKE_VERSION-7}"
	;;
"secrets versions access")
	if [[ ${FAKE_GCLOUD_RC:-0} -ne 0 ]]; then
		printf 'partial'
		exit "${FAKE_GCLOUD_RC}"
	fi
	printf 'django_username: "admin"\ndjango_password: "not-a-real-password"\n'
	;;
"secrets versions list")
	[[ ${FAKE_LIST_RC:-0} -eq 0 ]] || exit "${FAKE_LIST_RC}"
	printf '%s' "${FAKE_ENABLED:-}"
	;;
"secrets versions disable")
	[[ $4 != "${FAKE_DISABLE_FAIL:-}" ]] || exit 1
	# A boot killed while it disables the version
	[[ $4 != "${FAKE_DISABLE_KILL:-}" ]] || kill -KILL "${PPID}"
	;;
*)
	exit 64
	;;
esac
"""

# Logs each call. The commands given on stdin are saved as call-<n> in
# FAKE_STDIN_DIR (when set), and call <n> exits with the n-th number in
# FAKE_SUDO_RCS (default 0). When FAKE_STARTED_MARKER (FAKE_FETCHED_MARKER)
# is set, whether that file exists at each call is saved as started-<n>
# (fetched-<n>).
SUDO_FAKE = r"""#!/bin/bash
(IFS=$'\x1f'; printf 'sudo\x1f%s\n' "$*") >>"${FAKE_LOG}"
n=$(grep -c '^sudo' "${FAKE_LOG}")
if [[ -n ${FAKE_STDIN_DIR:-} ]]; then
	cat >"${FAKE_STDIN_DIR}/call-${n}"
else
	cat >/dev/null
fi
for marker in started fetched; do
	var="FAKE_${marker^^}_MARKER"
	if [[ -n ${!var:-} ]]; then
		if [[ -f ${!var} ]]; then
			echo yes >"${FAKE_STDIN_DIR}/${marker}-${n}"
		else
			echo no >"${FAKE_STDIN_DIR}/${marker}-${n}"
		fi
	fi
done
read -r -a rcs <<<"${FAKE_SUDO_RCS:-}"
exit "${rcs[n - 1]:-0}"
"""

# Logs each call. is-active answers FAKE_ACTIVE_RC (default 0, active), and
# disable FAKE_DISABLE_NOW_RC (default 0).
SYSTEMCTL_FAKE = r"""#!/bin/bash
(IFS=$'\x1f'; printf 'systemctl\x1f%s\n' "$*") >>"${FAKE_LOG}"
case $1 in
is-active) exit "${FAKE_ACTIVE_RC:-0}" ;;
disable) exit "${FAKE_DISABLE_NOW_RC:-0}" ;;
*) exit 64 ;;
esac
"""

# Logs each call. Call <n> prints the n-th entry of FAKE_SUPERVISOR_STATES
# (the last repeats): one line per process for states joined by "+", nothing
# for EMPTY, and a connection error (on standard output, as supervisorctl
# prints it) for SOCKET. FAKE_SUPERVISOR_WARNING, when set, is printed on
# standard error. Exits with FAKE_SUPERVISOR_RC (default 0), as the exit
# status differs between supervisor versions.
SUPERVISORCTL_FAKE = r"""#!/bin/bash
(IFS=$'\x1f'; printf 'supervisorctl\x1f%s\n' "$*") >>"${FAKE_LOG}"
n=$(grep -c '^supervisorctl' "${FAKE_LOG}")
read -r -a entries <<<"${FAKE_SUPERVISOR_STATES:-RUNNING}"
entry=${entries[n - 1]:-${entries[-1]}}
[[ -z ${FAKE_SUPERVISOR_WARNING:-} ]] || echo "${FAKE_SUPERVISOR_WARNING}" >&2
if [[ ${entry} == SOCKET ]]; then
	echo "unix:///run/supervisor/supervisor.sock no such file"
elif [[ ${entry} != EMPTY ]]; then
	IFS=+ read -r -a states <<<"${entry}"
	for i in "${!states[@]}"; do
		printf 'gcluster-uvicorn-background:gcluster-uvicorn-background_%02d   %s   pid 42, uptime 0:00:05\n' "${i}" "${states[i]}"
	done
fi
exit "${FAKE_SUPERVISOR_RC:-0}"
"""

# Logs each call and returns at once.
SLEEP_FAKE = r"""#!/bin/bash
(IFS=$'\x1f'; printf 'sleep\x1f%s\n' "$*") >>"${FAKE_LOG}"
"""

# Records the mode of the file it is asked to read.
YQ_FAKE = r"""#!/bin/bash
stat -c '%a' "${@: -1}" >>"${FAKE_YQ_MODES}"
echo value
"""


def config_section(script):
    """The first-boot lines that read the configuration, from the metadata
    lookup to the removal of the copy once its values are loaded."""
    start = script.index("\nconfig_secret=$(curl") + 1
    loaded = script.index("repo_branch=$(")
    end = script.index(f"rm -f {CONFIG_PATH}\n", loaded) + len(f"rm -f {CONFIG_PATH}\n")
    return script[start:end]


def installed_section(script):
    """The lines an installed server runs to disable the configuration
    secret's enabled versions, before it prepares stored credentials."""
    start = script.index("\tinstalled_config_secret=$(curl")
    end = script.index("\t# Stored credentials are prepared on every start", start)
    return script[start:end]


def installed_branch(script):
    """Everything an installed server runs, from its metadata lookup to its
    exit."""
    start = script.index("\tinstalled_config_secret=$(curl")
    exiting = script.index('\tprintf "Exiting...\\n"\n', start)
    end = script.index("\texit 0\n", exiting) + len("\texit 0\n")
    return script[start:end]


def finished_install_section(script):
    """The first-boot lines, after the server is started, that disable the
    version of the configuration secret read earlier and record that the
    first boot is complete, or report why it is not."""
    start = script.index("\n# Once the server is installed and running") + 1
    last = script.index("\n\tif [[ ! -d ${tkfe_code_dir} ]]; then\n", start)
    end = script.index("\nfi\n", last) + len("\nfi\n")
    return script[start:end]


def started_marker_section(script):
    """The first-boot lines that record that the first boot has started"""
    start = script.index("\n# Record that the first boot has started") + 1
    end = script.index("\nfi\n", start) + len("\nfi\n")
    return script[start:end]


def fetch_section(script):
    """The first-boot lines that fetch the TKFE code, as gcluster, and
    remove what a failed fetch left"""
    fetch = script.index("  ${fetch_hpc_toolkit}\nEOF\n")
    start = script.rindex("sudo su - gcluster -c /bin/bash <<EOF\n", 0, fetch)
    removal = script.index('rm -rf "${tkfe_code_dir:?}"', script.index("fetch_rc=$?\n", start))
    end = script.index("\nfi\n", removal) + len("\nfi\n")
    return script[start:end]


def top_section(script):
    """The lines every boot runs first: removing a partial copy of the code,
    then the installed branch, to its exit"""
    start = script.index("\n# A first boot interrupted while it fetched the code") + 1
    end = script.index('\tprintf "Exiting...\\n"\n\texit 0\nfi\n', start) + len(
        '\tprintf "Exiting...\\n"\n\texit 0\nfi\n'
    )
    return script[start:end]


def go_path_section(script):
    """The lines that add Go's folders to every user's PATH"""
    start = script.index("\ngo_path_line=") + 1
    end = script.index(">>/etc/bashrc\n", start) + len(">>/etc/bashrc\n")
    return script[start:end]


def setup_section(script):
    """The first-boot lines that set TKFE up, as gcluster"""
    start = script.index("\n# Any failed step fails the set-up") + 1
    end = script.index("setup_rc=$?\n", start) + len("setup_rc=$?\n")
    return script[start:end]


def heredoc_body(section):
    """The commands a section's first sudo heredoc gives the shell"""
    start = section.index("<<EOF\n") + len("<<EOF\n")
    return section[start:section.index("\nEOF\n", start)]


def write_installed_branch(tmp, started, complete):
    """A script, in tmp, of the installed branch with its markers at started
    and complete and the installed code's bootstrap command present"""
    bootstrap = tmp / "ofe_bootstrap_credentials.py"
    bootstrap.write_text("")
    branch = installed_branch(STARTUP.read_text()).replace(BOOTSTRAP_PY, str(bootstrap))
    script = tmp / "branch.sh"
    script.write_text(
        f'GCP_PROJECT="{PROJECT}"\nfirst_boot_started="{started}"\n'
        f'first_boot_complete="{complete}"\n{branch}'
    )
    return script


def disable_call(version):
    return [
        "secrets", "versions", "disable", version,
        f"--secret={SECRET}", f"--project={PROJECT}", "--quiet",
    ]


class StartupScriptTextTests(SimpleTestCase):
    """The script reads the secret and never the bucket object"""

    def setUp(self):
        self.script = STARTUP.read_text()

    def test_never_reads_or_deletes_the_bucket_object(self):
        self.assertNotIn("webserver/config", self.script)

    def test_reads_the_secret_named_in_its_metadata(self):
        self.assertIn("attributes/ofe-server-config-secret", self.script)
        self.assertRegex(
            self.script,
            r'gcloud secrets versions describe latest \\\n\s*'
            r'--secret="\$\{config_secret\}" --project="\$\{GCP_PROJECT\}"',
        )
        self.assertRegex(
            self.script,
            r'gcloud secrets versions access "\$\{config_version\}" \\\n\s*'
            r'--secret="\$\{config_secret\}" --project="\$\{GCP_PROJECT\}" >/tmp/config',
        )

    def test_copy_is_private_and_removed_once_read(self):
        section = config_section(self.script)
        self.assertIn("umask 077", section)
        # Removed before it is written (an earlier copy keeps its mode), when
        # it cannot be read, and after its values are loaded.
        self.assertEqual(section.count(f"rm -f {CONFIG_PATH}"), 3)
        after = self.script[self.script.index(section) + len(section):]
        self.assertNotIn(CONFIG_PATH, after)

    def test_installed_server_disables_versions_before_it_exits(self):
        # The step is in the branch an installed server runs, before its exit.
        installed = self.script.index(INSTALLED_CHECK)
        section = self.script.index(installed_section(self.script))
        self.assertLess(installed, section)
        self.assertLess(section, self.script.index('printf "Exiting...\\n"'))
        self.assertLess(section, self.script.index(config_section(self.script)))

    def test_reading_the_configuration_never_disables_its_version(self):
        # A boot that stops anywhere before the install finished leaves the
        # version enabled, so the next boot reads it again.
        self.assertNotIn("versions disable", config_section(self.script))

    def test_version_is_disabled_only_after_the_server_is_installed_and_started(self):
        disable = self.script.index(finished_install_section(self.script))
        # The one first-boot disable, after the toolkit is fetched (cloned or
        # unpacked), its packages are installed and the server is started.
        first_boot = self.script[self.script.index('printf "Exiting...\\n"'):]
        self.assertEqual(first_boot.count('gcloud secrets versions disable "${config_version}"'), 1)
        # Only when the server is running
        self.assertLess(
            self.script.index("systemctl is-active --quiet gcluster.service"),
            self.script.index('gcloud secrets versions disable "${config_version}"', disable),
        )
        for step in (
            "\n  ${fetch_hpc_toolkit}\n",
            "pip install --require-hashes -r /opt/gcluster/cluster-toolkit/community/front-end/ofe/requirements.txt",
            "\nsystemctl start gcluster.service\n",
        ):
            with self.subTest(step=step):
                self.assertLess(self.script.index(step), disable)

    def test_the_application_polled_is_the_one_supervisord_runs(self):
        program = self.script.index("[program:gcluster-uvicorn-background]")
        section = finished_install_section(self.script)
        self.assertLess(program, self.script.index(section))
        self.assertIn("supervisorctl status 'gcluster-uvicorn-background:*'", section)

    def test_a_failed_credential_set_up_is_an_error(self):
        line = (
            "python manage.py ofe_bootstrap_credentials || printf \"ERROR: preparing stored"
            " credentials failed; see the messages above. TKFE shows the recorded errors to"
            " administrators until a start succeeds\\n\""
        )
        # On an installed server's start and on the first boot
        self.assertEqual(self.script.count(line), 2)
        self.assertNotIn("WARNING: preparing stored credentials failed", self.script)

    def test_paths_are_set_before_the_installed_branch(self):
        # The run tests set these variables themselves.
        self.assertEqual(self.script.count(PATH_LINES), 1)
        self.assertLess(
            self.script.index(PATH_LINES),
            self.script.index(INSTALLED_CHECK),
        )

    def test_installed_server_checks_its_markers_before_anything_else(self):
        branch = installed_branch(self.script)
        check = branch.index("if [[ ! -f ${first_boot_complete} && -f ${first_boot_started} ]]; then")
        self.assertLess(check, branch.index("gcloud secrets versions list"))
        self.assertLess(check, branch.index("ofe_bootstrap_credentials"))
        # The markers decide, not the web server's unit file
        self.assertNotIn("/etc/systemd/system/gcluster.service", branch)

    def test_started_marker_is_written_before_the_code_is_fetched(self):
        started = self.script.index(started_marker_section(self.script))
        self.assertLess(started, self.script.index("if id gcluster >/dev/null 2>&1; then"))
        self.assertLess(started, self.script.index(fetch_section(self.script)))
        self.assertLess(
            self.script.index("# Install go version"), self.script.index(setup_section(self.script))
        )

    def test_fetched_marker_is_written_after_the_fetch_and_before_the_set_up(self):
        fetch = fetch_section(self.script)
        written = self.script.index('touch "${first_boot_fetched}"')
        self.assertLess(self.script.index("  ${fetch_hpc_toolkit}\nEOF\n"), written)
        self.assertLess(written, self.script.index(setup_section(self.script)))
        self.assertIn('touch "${first_boot_fetched}"', fetch)
        self.assertEqual(self.script.count('touch "${first_boot_fetched}"'), 1)

    def test_a_partial_copy_is_removed_before_the_installed_check(self):
        self.assertLess(
            self.script.index(top_section(self.script)), self.script.index(INSTALLED_CHECK)
        )
        self.assertNotIn("if [[ -d /opt/gcluster/cluster-toolkit ]]", self.script)

    def test_go_tools_are_pinned(self):
        self.assertNotRegex(self.script, r"go install \S+@latest")
        self.assertIn("go install github.com/google/addlicense@v1.2.0\n", self.script)

    def test_go_path_is_added_to_bashrc_once(self):
        section = go_path_section(self.script)
        self.assertIn(
            'grep -qxF "${go_path_line}" /etc/bashrc || printf \'%s\\n\' "${go_path_line}"'
            " >>/etc/bashrc\n",
            section,
        )
        self.assertEqual(self.script.count(">>/etc/bashrc"), 1)

    def test_complete_marker_is_written_before_the_disable(self):
        # A boot stopped between the two leaves a server that starts normally
        # and disables the version then.
        section = finished_install_section(self.script)
        self.assertLess(
            section.index('touch "${first_boot_complete}"'),
            section.index('gcloud secrets versions disable "${config_version}"'),
        )
        # Nowhere else
        self.assertEqual(self.script.count('touch "${first_boot_complete}"'), 1)

    def test_fetch_and_set_up_stop_at_a_failed_step(self):
        for section in (fetch_section(self.script), setup_section(self.script)):
            with self.subTest(section=section[:60]):
                self.assertTrue(heredoc_body(section).startswith("  set -e\n"))
        # Preparing stored credentials is the one step allowed to fail.
        body = heredoc_body(setup_section(self.script))
        self.assertIn("python manage.py ofe_bootstrap_credentials || printf", body)


class StartupScriptRunTests(SimpleTestCase):
    """The configuration section, run against fake commands"""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-startup-test-"))
        self.addCleanup(shutil.rmtree, self.tmp)
        self.bin = self.tmp / "bin"
        self.bin.mkdir()
        for name, content in (("curl", CURL_FAKE), ("gcloud", GCLOUD_FAKE), ("yq", YQ_FAKE)):
            (self.bin / name).write_text(content)
            (self.bin / name).chmod(0o755)
        self.config = self.tmp / "config"
        self.log = self.tmp / "calls.log"
        self.modes = self.tmp / "modes"
        section = config_section(STARTUP.read_text())
        section = section.replace(CONFIG_PATH, str(self.config)).replace(YQ, str(self.bin / "yq"))
        self.script = self.tmp / "section.sh"
        self.script.write_text(f'GCP_PROJECT="{PROJECT}"\n{section}echo finished\n')
        self.env = dict(
            os.environ,
            PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
            FAKE_SECRET=SECRET,
            FAKE_LOG=str(self.log),
            FAKE_YQ_MODES=str(self.modes),
        )

    def run_section(self):
        return subprocess.run(
            ["bash", "-c", f"umask 022; exec bash {self.script}"],
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
            timeout=60,
        )

    def gcloud_calls(self):
        if not self.log.exists():
            return []
        return [line.split("\x1f")[1:] for line in self.log.read_text().splitlines()]

    def test_reads_the_newest_version_of_its_secret_and_leaves_it_enabled(self):
        result = self.run_section()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("finished", result.stdout)
        # The version is resolved, then read; it is disabled only once the
        # server is installed (FinishedInstallRunTests).
        self.assertEqual(
            self.gcloud_calls(),
            [
                [
                    "secrets", "versions", "describe", "latest", f"--secret={SECRET}",
                    f"--project={PROJECT}", "--format=value(name.basename())",
                ],
                ["secrets", "versions", "access", VERSION, f"--secret={SECRET}", f"--project={PROJECT}"],
            ],
        )

    def test_copy_is_readable_only_by_its_owner_and_then_removed(self):
        result = self.run_section()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        modes = self.modes.read_text().split()
        self.assertTrue(modes)
        self.assertEqual(set(modes), {"600"})
        self.assertFalse(self.config.exists())

    def test_an_earlier_world_readable_copy_is_replaced(self):
        self.config.write_text("old")
        self.config.chmod(0o644)
        result = self.run_section()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(set(self.modes.read_text().split()), {"600"})

    def test_stops_when_the_secret_cannot_be_read(self):
        self.env["FAKE_GCLOUD_RC"] = "1"
        result = self.run_section()
        self.assertEqual(result.returncode, 1)
        self.assertIn(
            f"ERROR: could not read the server configuration from secret {SECRET}\n", result.stdout
        )
        # A version a completed install disabled is a likely cause; the
        # message gives the command that enables it again.
        self.assertIn(
            f"A completed install disables the version it read; if version {VERSION} is"
            f" disabled, enable it again with: gcloud secrets versions enable {VERSION}"
            f" --secret={SECRET} --project={PROJECT}\n",
            result.stdout,
        )
        self.assertNotIn("finished", result.stdout)
        self.assertFalse(self.config.exists())
        self.assertFalse(self.modes.exists())
        self.assertNotIn("disable", [call[2] for call in self.gcloud_calls()])

    def test_a_disable_that_would_fail_is_never_tried_while_reading(self):
        # Reading does not depend on disabling: the boot goes on to install,
        # and the version stays enabled for a boot that has to try again.
        self.env["FAKE_DISABLE_FAIL"] = VERSION
        result = self.run_section()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("finished", result.stdout)
        self.assertNotIn("disable", [call[2] for call in self.gcloud_calls()])
        self.assertFalse(self.config.exists())

    def test_stops_when_the_newest_version_cannot_be_found(self):
        for env in ({"FAKE_DESCRIBE_RC": "1"}, {"FAKE_VERSION": ""}):
            with self.subTest(env=env):
                self.log.unlink(missing_ok=True)
                result = subprocess.run(
                    ["bash", str(self.script)],
                    env={**self.env, **env},
                    capture_output=True,
                    text=True,
                    check=False,
                    timeout=60,
                )
                self.assertEqual(result.returncode, 1)
                self.assertIn(f"could not find the newest version of secret {SECRET}", result.stdout)
                self.assertEqual([call[2] for call in self.gcloud_calls()], ["describe"])
                self.assertFalse(self.config.exists())

    def test_stops_when_the_metadata_key_is_missing(self):
        self.env["FAKE_SECRET"] = ""
        result = self.run_section()
        self.assertEqual(result.returncode, 1)
        self.assertIn("ofe-server-config-secret is not set", result.stdout)
        self.assertEqual(self.gcloud_calls(), [])
        self.assertFalse(self.config.exists())


class InstalledServerRunTests(SimpleTestCase):
    """An installed server's start disables enabled versions, never fatally"""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-startup-test-"))
        self.addCleanup(shutil.rmtree, self.tmp)
        self.bin = self.tmp / "bin"
        self.bin.mkdir()
        for name, content in (("curl", CURL_FAKE), ("gcloud", GCLOUD_FAKE)):
            (self.bin / name).write_text(content)
            (self.bin / name).chmod(0o755)
        self.log = self.tmp / "calls.log"
        # A completed first boot
        (self.tmp / "first-boot-started").touch()
        (self.tmp / "first-boot-complete").touch()
        self.script = self.tmp / "installed.sh"
        self.script.write_text(
            f'GCP_PROJECT="{PROJECT}"\n'
            f'first_boot_started="{self.tmp}/first-boot-started"\n'
            f'first_boot_complete="{self.tmp}/first-boot-complete"\n'
            f"{installed_section(STARTUP.read_text())}echo finished\n"
        )
        self.env = dict(
            os.environ,
            PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
            FAKE_SECRET=SECRET,
            FAKE_LOG=str(self.log),
            FAKE_ENABLED="3\n5\n",
        )

    def run_section(self):
        result = subprocess.run(
            ["bash", str(self.script)],
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
            timeout=60,
        )
        # The server is installed and must still start.
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("finished", result.stdout)
        return result

    def gcloud_calls(self):
        if not self.log.exists():
            return []
        return [line.split("\x1f")[1:] for line in self.log.read_text().splitlines()]

    def test_disables_every_enabled_version(self):
        result = self.run_section()
        self.assertEqual(
            self.gcloud_calls(),
            [
                [
                    "secrets", "versions", "list", SECRET, f"--project={PROJECT}",
                    "--filter=state:ENABLED", "--format=value(name.basename())",
                ],
                disable_call("3"),
                disable_call("5"),
            ],
        )
        self.assertIn(f"Disabled version 3 of secret {SECRET}", result.stdout)
        self.assertIn(f"Disabled version 5 of secret {SECRET}", result.stdout)

    def test_nothing_to_disable(self):
        self.env["FAKE_ENABLED"] = ""
        self.run_section()
        self.assertEqual([call[2] for call in self.gcloud_calls()], ["list"])

    def test_a_failed_disable_is_reported_and_the_rest_are_disabled(self):
        self.env["FAKE_DISABLE_FAIL"] = "3"
        result = self.run_section()
        self.assertIn(
            f"WARNING: could not disable version 3 of secret {SECRET}, which holds the admin"
            " password; disable it by hand",
            result.stdout,
        )
        self.assertEqual(self.gcloud_calls()[1:], [disable_call("3"), disable_call("5")])

    def test_a_failed_list_is_reported(self):
        self.env["FAKE_LIST_RC"] = "1"
        result = self.run_section()
        self.assertIn(f"WARNING: could not list the versions of secret {SECRET}", result.stdout)
        self.assertEqual([call[2] for call in self.gcloud_calls()], ["list"])

    def test_no_secret_in_metadata_calls_nothing(self):
        # A server whose deployment has not been upgraded has no secret.
        self.env["FAKE_SECRET"] = ""
        self.run_section()
        self.assertEqual(self.gcloud_calls(), [])


class FirstBootRunTests(SimpleTestCase):
    """The first boot's marker, fetch, set-up and finish, run in order
    against fake sudo, systemctl and gcloud"""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-startup-test-"))
        self.addCleanup(shutil.rmtree, self.tmp)
        self.bin = self.tmp / "bin"
        self.bin.mkdir()
        for name, content in (
            ("curl", CURL_FAKE), ("gcloud", GCLOUD_FAKE), ("sudo", SUDO_FAKE),
            ("systemctl", SYSTEMCTL_FAKE), ("supervisorctl", SUPERVISORCTL_FAKE),
            ("sleep", SLEEP_FAKE),
        ):
            (self.bin / name).write_text(content)
            (self.bin / name).chmod(0o755)
        self.log = self.tmp / "calls.log"
        self.stdin = self.tmp / "stdin"
        self.stdin.mkdir()
        self.state = self.tmp / "state"
        self.code = self.tmp / "cluster-toolkit"
        self.started = self.state / "first-boot-started"
        self.fetched = self.state / "first-boot-fetched"
        self.complete = self.state / "first-boot-complete"
        self.script = self.tmp / "first-boot.sh"
        self.write_script()
        self.env = dict(
            os.environ,
            PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
            FAKE_LOG=str(self.log),
            FAKE_STDIN_DIR=str(self.stdin),
            FAKE_STARTED_MARKER=str(self.started),
            FAKE_FETCHED_MARKER=str(self.fetched),
        )

    def write_script(self, app_wait_polls=None):
        script = STARTUP.read_text()
        if app_wait_polls is not None:
            self.assertEqual(script.count("\napp_wait_polls=30\n"), 1)
            script = script.replace("\napp_wait_polls=30\n", f"\napp_wait_polls={app_wait_polls}\n")
        # The values set earlier in the same run
        self.script.write_text(
            f'GCP_PROJECT="{PROJECT}"\nconfig_secret="{SECRET}"\nconfig_version="{VERSION}"\n'
            f'tkfe_code_dir="{self.code}"\nofe_state_dir="{self.state}"\n'
            f'first_boot_started="{self.started}"\nfirst_boot_fetched="{self.fetched}"\n'
            f'first_boot_complete="{self.complete}"\n'
            'fetch_hpc_toolkit="true"\n'
            f"{started_marker_section(script)}{fetch_section(script)}"
            f"{setup_section(script)}{finished_install_section(script)}echo finished\n"
        )

    def run_script(self, sudo_rcs="0 0", code_fetched=True):
        """sudo_rcs: the exit status of the fetch and of the set-up"""
        if code_fetched:
            # What the fetch leaves (a partial extract when it fails)
            self.code.mkdir()
        self.env["FAKE_SUDO_RCS"] = sudo_rcs
        return subprocess.run(
            ["bash", str(self.script)],
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
            timeout=60,
        )

    def run_boot(self, sudo_rcs="0 0", code_fetched=True, completes=True):
        result = self.run_script(sudo_rcs, code_fetched)
        if completes:
            # The script goes on to its end (Grafana, certbot).
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("finished", result.stdout)
        else:
            # A failed install stops right after its ERROR: nothing after the
            # finish block (Grafana, certbot) runs.
            self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
            self.assertNotIn("finished", result.stdout)
            self.assertTrue(
                result.stdout.rstrip("\n").splitlines()[-1].startswith(
                    "ERROR: the first boot did not finish installing TKFE ("
                ),
                result.stdout,
            )
        # Both steps ran, after the started marker was written
        self.assertEqual(len(self.calls("sudo")), 2)
        for n in (1, 2):
            self.assertEqual((self.stdin / f"started-{n}").read_text(), "yes\n")
        return result

    def calls(self, command):
        if not self.log.exists():
            return []
        return [
            fields[1:]
            for fields in (line.split("\x1f") for line in self.log.read_text().splitlines())
            if fields[0] == command
        ]

    def gcloud_calls(self):
        return self.calls("gcloud")

    def failure(self, part, advice, web_server="the web server has been stopped and disabled"):
        return (
            f"ERROR: the first boot did not finish installing TKFE ({part}); version {VERSION}"
            f" of secret {SECRET} stays enabled, and {web_server}. {advice}"
        )

    def assert_web_server_stopped(self):
        # nginx, the only public listener, is stopped and kept from starting
        self.assertIn(["disable", "--now", "gcluster.service"], self.calls("systemctl"))

    def test_a_complete_install_records_it_then_disables_the_version(self):
        result = self.run_boot()
        # The web server is left running
        self.assertEqual(self.calls("systemctl"), [["is-active", "--quiet", "gcluster.service"]])
        self.assertEqual(self.gcloud_calls(), [disable_call(VERSION)])
        self.assertTrue(self.started.exists())
        self.assertTrue(self.fetched.exists())
        self.assertTrue(self.complete.exists())
        self.assertNotIn("ERROR", result.stdout)

    def test_the_fetched_marker_is_written_after_the_fetch_and_before_the_set_up(self):
        self.state.mkdir()
        # An old one is removed when the first boot starts
        self.fetched.touch()
        self.run_boot()
        self.assertEqual((self.stdin / "fetched-1").read_text(), "no\n")
        self.assertEqual((self.stdin / "fetched-2").read_text(), "yes\n")

    def test_a_failed_fetch_writes_no_fetched_marker(self):
        self.run_boot(completes=False, sudo_rcs="1 1")
        self.assertFalse(self.fetched.exists())

    def test_a_fetched_marker_that_cannot_be_written_counts_as_a_failed_fetch(self):
        # Its directory is missing
        self.fetched = self.state / "missing" / "first-boot-fetched"
        self.write_script()
        result = self.run_boot(completes=False)
        self.assertIn(f"ERROR: could not write {self.fetched}\n", result.stdout)
        self.assertFalse(self.code.exists())
        self.assertFalse(self.complete.exists())
        self.assertIn(
            self.failure(
                "fetching the TKFE code failed",
                "No TKFE code was installed, so restarting the server VM tries again",
            ),
            result.stdout,
        )

    def test_a_web_server_that_cannot_be_stopped_is_reported(self):
        self.env["FAKE_DISABLE_NOW_RC"] = "1"
        result = self.run_boot(completes=False, sudo_rcs="0 1")
        self.assert_web_server_stopped()
        self.assertIn(
            "ERROR: could not stop the web server; run: systemctl disable --now"
            " gcluster.service\n",
            result.stdout,
        )
        self.assertIn(
            self.failure(
                "setting up TKFE failed",
                "Recreate the server VM",
                web_server="the web server could not be stopped (see above)",
            ),
            result.stdout,
        )
        self.assertFalse(self.complete.exists())

    def test_steps_run_with_set_e(self):
        self.run_boot()
        for n in (1, 2):
            self.assertTrue((self.stdin / f"call-{n}").read_text().startswith("  set -e\n"))

    def test_a_failed_disable_still_records_the_install(self):
        self.env["FAKE_DISABLE_FAIL"] = VERSION
        result = self.run_boot()
        self.assertIn(
            f"ERROR: could not disable version {VERSION} of secret {SECRET}, which holds the"
            " admin password; the next start disables it, or run: gcloud secrets versions"
            f" disable {VERSION} --secret={SECRET} --project={PROJECT}",
            result.stdout,
        )
        self.assertEqual(self.gcloud_calls(), [disable_call(VERSION)])
        self.assertTrue(self.complete.exists())

    def test_a_boot_killed_during_the_disable_leaves_a_server_that_starts(self):
        self.env["FAKE_DISABLE_KILL"] = VERSION
        result = self.run_script()
        self.assertEqual(result.returncode, -9, result.stdout + result.stderr)
        self.assertEqual(self.gcloud_calls(), [disable_call(VERSION)])
        self.assertTrue(self.complete.exists())
        # The next start takes the normal path, and disables the version then
        later = self.tmp / "later"
        later.mkdir()
        branch = write_installed_branch(later, self.started, self.complete)
        log = later / "calls.log"
        started = subprocess.run(
            ["bash", str(branch)],
            env={
                **self.env, "FAKE_LOG": str(log), "FAKE_SECRET": SECRET,
                "FAKE_ENABLED": f"{VERSION}\n", "FAKE_DISABLE_KILL": "",
            },
            capture_output=True,
            text=True,
            check=False,
            timeout=60,
            stdin=subprocess.DEVNULL,
        )
        self.assertEqual(started.returncode, 0, started.stdout + started.stderr)
        self.assertNotIn("ERROR", started.stdout)
        self.assertIn(f"Disabled version {VERSION} of secret {SECRET}", started.stdout)
        self.assertIn("Exiting...", started.stdout)

    def test_a_complete_marker_that_cannot_be_written_is_reported(self):
        # Its directory is missing
        self.complete = self.state / "missing" / "first-boot-complete"
        self.write_script()
        result = self.run_boot()
        self.assertIn(
            f"ERROR: could not write {self.complete}, which records that the first boot is"
            f" complete, so version {VERSION} of secret {SECRET} stays enabled; run: sudo touch"
            f" {self.complete}, then restart the server VM\n",
            result.stdout,
        )
        # Not disabled: until the marker exists, later starts report a
        # half-installed server whose versions stay enabled
        self.assertEqual(self.gcloud_calls(), [])

    def test_a_started_marker_that_cannot_be_written_stops_before_the_fetch(self):
        blocker = self.tmp / "a-file"
        blocker.write_text("")
        cases = {
            # Its directory cannot be made
            "mkdir": (blocker / "state", None),
            # An old complete marker cannot be removed
            "rm": (self.state, "complete"),
            # Nor an old fetched marker
            "rm fetched": (self.tmp / "state2", "fetched"),
        }
        for case, (state, directory) in cases.items():
            with self.subTest(case):
                self.log.unlink(missing_ok=True)
                self.state = state
                self.started = state / "first-boot-started"
                self.fetched = state / "first-boot-fetched"
                self.complete = state / "first-boot-complete"
                if directory:
                    getattr(self, directory).mkdir(parents=True)
                self.write_script()
                result = self.run_script(code_fetched=False)
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertIn(
                    f"ERROR: could not write {self.started}; nothing has been installed,"
                    " restart the server VM to try again",
                    result.stdout,
                )
                self.assertEqual(self.calls("sudo"), [])
                self.assertEqual(self.gcloud_calls(), [])
                self.assertNotIn("finished", result.stdout)

    def test_a_failed_fetch_leaves_no_code_and_a_restart_retries(self):
        result = self.run_boot(completes=False, sudo_rcs="1 1")
        self.assertIn("ERROR: fetching the TKFE code failed", result.stdout)
        self.assertFalse(self.code.exists())
        self.assertEqual(self.gcloud_calls(), [])
        self.assertFalse(self.complete.exists())
        self.assertIn(
            self.failure(
                "fetching the TKFE code failed",
                "No TKFE code was installed, so restarting the server VM tries again",
            ),
            result.stdout,
        )
        self.assert_web_server_stopped()

    def test_a_failed_set_up_with_the_server_running_needs_a_new_vm(self):
        result = self.run_boot(completes=False, sudo_rcs="0 1")
        self.assertTrue(self.code.exists())
        self.assertEqual(self.gcloud_calls(), [])
        self.assertFalse(self.complete.exists())
        self.assertIn(
            self.failure("setting up TKFE failed", "Recreate the server VM"), result.stdout
        )
        self.assert_web_server_stopped()

    def test_a_server_that_is_not_running_needs_a_new_vm(self):
        self.env["FAKE_ACTIVE_RC"] = "3"
        result = self.run_boot(completes=False)
        self.assertEqual(self.gcloud_calls(), [])
        self.assertFalse(self.complete.exists())
        self.assertIn(
            self.failure("the gcluster service is not running", "Recreate the server VM"),
            result.stdout,
        )
        self.assert_web_server_stopped()
        # The application is not waited for
        self.assertEqual(self.calls("supervisorctl"), [])
        self.assertEqual(self.calls("sleep"), [])

    def test_the_application_is_waited_for_until_it_is_running(self):
        self.env["FAKE_SUPERVISOR_STATES"] = "STARTING STARTING RUNNING"
        result = self.run_boot()
        self.assertEqual(
            self.calls("supervisorctl"), [["status", "gcluster-uvicorn-background:*"]] * 3
        )
        self.assertEqual(self.calls("sleep"), [["2"], ["2"]])
        self.assertTrue(self.complete.exists())
        self.assertEqual(self.gcloud_calls(), [disable_call(VERSION)])
        self.assertNotIn("ERROR", result.stdout)

    def test_the_exit_status_of_supervisorctl_is_not_trusted(self):
        # Some versions exit non-zero even when every process is RUNNING
        self.env["FAKE_SUPERVISOR_RC"] = "3"
        self.run_boot()
        self.assertEqual(len(self.calls("supervisorctl")), 1)
        self.assertTrue(self.complete.exists())
        self.assertEqual(self.gcloud_calls(), [disable_call(VERSION)])

    def test_the_wait_is_bounded(self):
        # The script's own limit: 30 polls, 2 s apart (60 s at most)
        self.env["FAKE_SUPERVISOR_STATES"] = "STARTING"
        self.run_boot(completes=False)
        self.assertEqual(len(self.calls("supervisorctl")), 30)
        self.assertEqual(self.calls("sleep"), [["2"]] * 29)

    def test_running_and_answering_at_the_first_poll_needs_no_wait(self):
        self.run_boot()
        self.assertEqual(len(self.calls("supervisorctl")), 1)
        self.assertEqual(
            self.calls("curl"),
            [["--silent", "--output", "/dev/null", "--max-time", "5", "http://127.0.0.1:8001/"]],
        )
        self.assertEqual(self.calls("sleep"), [])
        self.assertTrue(self.complete.exists())

    def test_the_application_is_waited_for_until_it_answers(self):
        # RUNNING from the start, but not answering on its port at first
        self.env["FAKE_APP_RCS"] = "7 7 0"
        result = self.run_boot()
        self.assertEqual(len(self.calls("supervisorctl")), 3)
        self.assertEqual(len(self.calls("curl")), 3)
        self.assertEqual(self.calls("sleep"), [["2"], ["2"]])
        self.assertTrue(self.complete.exists())
        self.assertEqual(self.gcloud_calls(), [disable_call(VERSION)])
        self.assertNotIn("ERROR", result.stdout)

    def test_a_warning_on_standard_error_does_not_count(self):
        self.env["FAKE_SUPERVISOR_WARNING"] = "Warning: something to note"
        result = self.run_boot()
        self.assertEqual(len(self.calls("supervisorctl")), 1)
        self.assertTrue(self.complete.exists())
        self.assertEqual(self.gcloud_calls(), [disable_call(VERSION)])
        self.assertNotIn("ERROR", result.stdout)

    def test_an_application_that_never_answers_needs_a_new_vm(self):
        # The reloader stays RUNNING while the application cannot load
        self.write_script(app_wait_polls=3)
        self.env["FAKE_APP_RCS"] = "7"
        result = self.run_boot(completes=False)
        self.assertEqual(len(self.calls("supervisorctl")), 3)
        self.assertEqual(len(self.calls("curl")), 3)
        self.assertEqual(self.gcloud_calls(), [])
        self.assertFalse(self.complete.exists())
        self.assertIn(
            self.failure("the TKFE application is not running", "Recreate the server VM"),
            result.stdout,
        )
        self.assert_web_server_stopped()

    def test_an_application_that_never_runs_needs_a_new_vm(self):
        for states in (
            "STARTING", "FATAL", "BACKOFF", "SOCKET", "EMPTY", "RUNNING+FATAL",
            "STARTING RUNNING",
        ):
            with self.subTest(states=states):
                self.log.unlink(missing_ok=True)
                shutil.rmtree(self.code, ignore_errors=True)
                # A shorter wait; the last poll may still see RUNNING
                self.write_script(app_wait_polls=1)
                self.env["FAKE_SUPERVISOR_STATES"] = states
                result = self.run_boot(completes=False)
                self.assertEqual(len(self.calls("supervisorctl")), 1)
                # Not asked to answer while not every process is RUNNING
                self.assertEqual(self.calls("curl"), [])
                self.assertEqual(self.calls("sleep"), [])
                self.assertEqual(self.gcloud_calls(), [])
                self.assertFalse(self.complete.exists())
                self.assertIn(
                    self.failure("the TKFE application is not running", "Recreate the server VM"),
                    result.stdout,
                )
                self.assert_web_server_stopped()

    def run_top(self):
        """The lines every boot runs first, with the paths of this test; a
        boot that falls through to the first boot prints first-boot-path"""
        bootstrap = self.tmp / "ofe_bootstrap_credentials.py"
        bootstrap.write_text("")
        top = top_section(STARTUP.read_text()).replace(BOOTSTRAP_PY, str(bootstrap))
        script = self.tmp / "top.sh"
        script.write_text(
            f'GCP_PROJECT="{PROJECT}"\ntkfe_code_dir="{self.code}"\n'
            f'first_boot_started="{self.started}"\nfirst_boot_fetched="{self.fetched}"\n'
            f'first_boot_complete="{self.complete}"\n{top}echo first-boot-path\n'
        )
        return subprocess.run(
            ["bash", str(script)],
            env={**self.env, "FAKE_SECRET": SECRET, "FAKE_ENABLED": f"{VERSION}\n"},
            capture_output=True,
            text=True,
            check=False,
            timeout=60,
            stdin=subprocess.DEVNULL,
        )

    def test_an_interrupted_fetch_is_removed_and_the_first_boot_runs_again(self):
        # A boot stopped during the clone: the started marker and part of
        # the code
        self.state.mkdir()
        self.started.touch()
        self.code.mkdir()
        (self.code / "partial").write_text("")
        result = self.run_top()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(
            "A previous first boot stopped while fetching the TKFE code; removing the partial"
            " copy and starting again\n",
            result.stdout,
        )
        self.assertFalse(self.code.exists())
        self.assertIn("first-boot-path", result.stdout)
        self.assertEqual(self.calls("gcloud"), [])
        # The first boot then runs and completes
        self.run_boot()
        self.assertTrue(self.complete.exists())
        self.assertEqual(self.gcloud_calls(), [disable_call(VERSION)])

    def test_a_fetched_but_unfinished_install_still_stops(self):
        self.state.mkdir()
        self.started.touch()
        self.fetched.touch()
        self.code.mkdir()
        result = self.run_top()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("ERROR: the first boot did not finish installing TKFE;", result.stdout)
        self.assertTrue(self.code.exists())
        self.assertNotIn("first-boot-path", result.stdout)
        self.assertEqual(self.calls("gcloud"), [])

    def test_a_server_from_an_earlier_tkfe_keeps_its_code(self):
        # No markers
        self.code.mkdir()
        result = self.run_top()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(self.code.exists())
        self.assertIn("Exiting...", result.stdout)
        self.assertNotIn("first-boot-path", result.stdout)
        self.assertNotIn("removing the partial copy", result.stdout)

    def test_a_complete_install_keeps_its_code(self):
        self.state.mkdir()
        for marker in (self.started, self.fetched, self.complete):
            marker.touch()
        self.code.mkdir()
        result = self.run_top()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(self.code.exists())
        self.assertIn("Exiting...", result.stdout)

    def test_a_complete_install_from_before_the_fetched_marker_keeps_its_code(self):
        # Installed by a script that wrote only the started and complete
        # markers
        self.state.mkdir()
        self.started.touch()
        self.complete.touch()
        self.code.mkdir()
        result = self.run_top()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(self.code.exists())
        self.assertNotIn("removing the partial copy", result.stdout)
        self.assertNotIn("first-boot-path", result.stdout)
        self.assertIn("Exiting...", result.stdout)
        self.assertEqual(
            [call[2:4] for call in self.calls("gcloud")],
            [["list", SECRET], ["disable", VERSION]],
        )

    def test_no_code_runs_the_first_boot(self):
        result = self.run_top()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("first-boot-path", result.stdout)
        self.assertNotIn("removing the partial copy", result.stdout)

    def test_an_old_complete_marker_is_removed_when_a_first_boot_starts(self):
        self.state.mkdir()
        self.complete.touch()
        self.run_boot(completes=False, sudo_rcs="1 1")
        self.assertFalse(self.complete.exists())


class InstalledBranchRunTests(SimpleTestCase):
    """A server whose first boot did not finish installing stops at each
    later boot, leaving the configuration readable for a new VM"""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-startup-test-"))
        self.addCleanup(shutil.rmtree, self.tmp)
        self.bin = self.tmp / "bin"
        self.bin.mkdir()
        for name, content in (
            ("curl", CURL_FAKE), ("gcloud", GCLOUD_FAKE), ("sudo", SUDO_FAKE),
            ("systemctl", SYSTEMCTL_FAKE),
        ):
            (self.bin / name).write_text(content)
            (self.bin / name).chmod(0o755)
        self.log = self.tmp / "calls.log"
        self.started = self.tmp / "first-boot-started"
        self.complete = self.tmp / "first-boot-complete"
        self.script = write_installed_branch(self.tmp, self.started, self.complete)
        self.env = dict(
            os.environ,
            PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
            FAKE_SECRET=SECRET,
            FAKE_LOG=str(self.log),
            FAKE_ENABLED="3\n5\n",
        )

    def run_branch(self):
        return subprocess.run(
            ["bash", str(self.script)],
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
            timeout=60,
        )

    def calls(self, command):
        if not self.log.exists():
            return []
        return [
            fields[1:]
            for fields in (line.split("\x1f") for line in self.log.read_text().splitlines())
            if fields[0] == command
        ]

    def assert_normal_start(self, result):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("ERROR", result.stdout)
        self.assertEqual(
            [call[2:4] for call in self.calls("gcloud")],
            [["list", SECRET], ["disable", "3"], ["disable", "5"]],
        )
        self.assertEqual(self.calls("sudo"), [["su", "-", "gcluster", "-c", "/bin/bash"]])
        self.assertIn("Exiting...", result.stdout)
        # The web server is left as it is
        self.assertEqual(self.calls("systemctl"), [])

    # The installed branch sees the code of an unfinished first boot only
    # once it was fetched: an interrupted fetch is removed before (see
    # FirstBootRunTests.test_an_interrupted_fetch_is_removed_...).

    def assert_web_server_stopped(self):
        # It may have been enabled before the first boot stopped
        self.assertEqual(self.calls("systemctl"), [["disable", "--now", "gcluster.service"]])

    def test_an_unfinished_first_boot_stops_without_disabling_or_preparing_credentials(self):
        self.started.touch()
        result = self.run_branch()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn(
            "ERROR: the first boot did not finish installing TKFE; this server cannot start,"
            " and the web server has been stopped and disabled. Recreate the server VM; the"
            f" enabled versions of secret {SECRET} stay enabled so the new VM can read its"
            " configuration\n",
            result.stdout,
        )
        self.assert_web_server_stopped()
        self.assertEqual(self.calls("gcloud"), [])
        self.assertEqual(self.calls("sudo"), [])
        self.assertNotIn("Exiting", result.stdout)

    def test_an_unfinished_first_boot_without_a_secret_in_metadata(self):
        self.started.touch()
        self.env["FAKE_SECRET"] = ""
        result = self.run_branch()
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn(
            "ERROR: the first boot did not finish installing TKFE; this server cannot start,"
            " and the web server has been stopped and disabled. Recreate the server VM\n",
            result.stdout,
        )
        self.assertNotIn("secret", result.stdout)
        self.assert_web_server_stopped()
        self.assertEqual(self.calls("gcloud"), [])
        self.assertEqual(self.calls("sudo"), [])

    def test_an_unfinished_first_boot_whose_web_server_cannot_be_stopped(self):
        self.started.touch()
        self.env["FAKE_DISABLE_NOW_RC"] = "1"
        result = self.run_branch()
        # Still a stop, with the command to run
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn(
            "ERROR: could not stop the web server; run: systemctl disable --now"
            " gcluster.service\n",
            result.stdout,
        )
        self.assertIn(
            "ERROR: the first boot did not finish installing TKFE; this server cannot start,"
            " and the web server could not be stopped (see above). Recreate the server VM;",
            result.stdout,
        )
        self.assert_web_server_stopped()
        self.assertEqual(self.calls("gcloud"), [])
        self.assertEqual(self.calls("sudo"), [])

    def test_a_complete_first_boot_starts_normally(self):
        self.started.touch()
        self.complete.touch()
        self.assert_normal_start(self.run_branch())

    def test_a_server_from_an_earlier_tkfe_starts_normally(self):
        # No markers: installed before they existed
        self.assert_normal_start(self.run_branch())


# Run in place of the commands the fetch and set-up give the shell. Each logs
# its name and arguments; a call whose arguments contain FAKE_FAIL_<NAME>
# fails. git clone makes a spack command, and python3.12 -m venv makes an
# activate script, as the real ones do.
STEP_FAKE = r"""#!/bin/bash
name=$(basename "$0")
(IFS=' '; printf '%s %s\n' "${name}" "$*") >>"${FAKE_STEPS}"
fail_var="FAKE_FAIL_${name//[^a-zA-Z0-9]/_}"
if [[ -n ${!fail_var:-} && "$*" == *"${!fail_var}"* ]]; then
	exit 1
fi
if [[ ${name} == git && $1 == clone && $* == *spack* ]]; then
	mkdir -p spack/bin
	printf '#!/bin/bash\n' >spack/bin/spack
	chmod +x spack/bin/spack
fi
if [[ ${name} == python3.12 && $1 == -m && $2 == venv ]]; then
	mkdir -p "$3/bin"
	: >"$3/bin/activate"
fi
exit 0
"""

# Runs the commands it is given on stdin, as the real sudo su - gcluster does.
SUDO_RUN_FAKE = r"""#!/bin/bash
exec bash
"""


class StepsRunTests(SimpleTestCase):
    """The fetch and set-up commands, run by bash with fake tools: a failed
    step stops the rest, except preparing stored credentials"""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="ofe-startup-test-"))
        self.addCleanup(shutil.rmtree, self.tmp)
        self.bin = self.tmp / "bin"
        self.bin.mkdir()
        (self.bin / "sudo").write_text(SUDO_RUN_FAKE)
        (self.bin / "sudo").chmod(0o755)
        for name in ("git", "go", "make", "python3.12", "pip", "python"):
            (self.bin / name).write_text(STEP_FAKE)
            (self.bin / name).chmod(0o755)
        # /opt/gcluster, with the fetched code
        self.root = self.tmp / "gcluster"
        self.ofe = self.root / "cluster-toolkit/community/front-end/ofe"
        (self.ofe / "website").mkdir(parents=True)
        (self.ofe / "website/nginx.conf").write_text("server_name SERVER_NAME;\n")
        self.steps = self.tmp / "steps"
        self.env = dict(
            os.environ,
            PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}",
            FAKE_STEPS=str(self.steps),
        )

    def run_section(self, section, rc_name):
        script = self.tmp / "section.sh"
        script.write_text(
            'fetch_hpc_toolkit="git clone -b main https://example.invalid/cluster-toolkit.git"\n'
            f'tkfe_code_dir="{self.tmp / "removed-code"}"\n'
            f'first_boot_fetched="{self.tmp / "first-boot-fetched"}"\n'
            + section.replace("/opt/gcluster", str(self.root))
            + f'echo "rc=${{{rc_name}}}"\n'
        )
        result = subprocess.run(
            ["bash", str(script)],
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
            timeout=60,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        steps = self.steps.read_text().splitlines() if self.steps.exists() else []
        return int(result.stdout.rsplit("rc=", 1)[1]), steps

    def run_setup(self):
        return self.run_section(setup_section(STARTUP.read_text()), "setup_rc")

    def test_a_complete_set_up(self):
        rc, steps = self.run_setup()
        self.assertEqual(rc, 0)
        self.assertEqual(steps[-1], "python manage.py seed_workbench_presets")
        # collectstatic never asks: its question would read the next line of
        # this script as the answer, cancel, and stop the set-up
        self.assertIn("python manage.py collectstatic --noinput", steps)
        # The last step, the nginx edit, ran too
        self.assertEqual((self.ofe / "website/nginx.conf").read_text(), "")

    def assert_nginx_untouched(self):
        self.assertIn("SERVER_NAME", (self.ofe / "website/nginx.conf").read_text())

    def test_a_failed_step_stops_the_set_up(self):
        self.env["FAKE_FAIL_pip"] = "django_requirements.txt"
        rc, steps = self.run_setup()
        self.assertNotEqual(rc, 0)
        self.assertTrue(steps[-1].startswith("pip install"), steps)
        self.assertIn("django_requirements.txt", steps[-1])
        # Nothing after it ran
        self.assertFalse(any(step.startswith("python manage.py") for step in steps), steps)
        self.assertFalse((self.ofe / "configuration.yaml").exists())
        self.assert_nginx_untouched()

    def test_a_failed_migrate_stops_the_set_up(self):
        self.env["FAKE_FAIL_python"] = "manage.py migrate"
        rc, steps = self.run_setup()
        self.assertNotEqual(rc, 0)
        self.assertEqual(steps[-1], "python manage.py migrate")
        self.assert_nginx_untouched()

    def assert_step_stops_the_set_up(self, step):
        self.env["FAKE_FAIL_python"] = step
        rc, steps = self.run_setup()
        self.assertNotEqual(rc, 0)
        self.assertEqual(steps[-1], f"python {step}")
        self.assert_nginx_untouched()

    # The steps after migrate stop it too, up to the last one

    def test_a_failed_collectstatic_stops_the_set_up(self):
        self.assert_step_stops_the_set_up("manage.py collectstatic --noinput")

    def test_a_failed_seed_stops_the_set_up(self):
        self.assert_step_stops_the_set_up("manage.py seed_workbench_presets")

    def test_failed_credential_preparation_does_not_stop_the_set_up(self):
        self.env["FAKE_FAIL_python"] = "ofe_bootstrap_credentials"
        rc, steps = self.run_setup()
        self.assertEqual(rc, 0)
        self.assertIn("python manage.py ofe_bootstrap_credentials", steps)
        self.assertEqual(steps[-1], "python manage.py seed_workbench_presets")
        self.assertEqual((self.ofe / "website/nginx.conf").read_text(), "")

    def test_a_fetch_that_cannot_reach_its_folder_fetches_nothing(self):
        # /opt/gcluster is missing, so cd fails and the clone never runs where
        # it should not.
        shutil.rmtree(self.root)
        rc, steps = self.run_section(fetch_section(STARTUP.read_text()), "fetch_rc")
        self.assertNotEqual(rc, 0)
        self.assertEqual(steps, [])

    def test_a_fetch(self):
        rc, steps = self.run_section(fetch_section(STARTUP.read_text()), "fetch_rc")
        self.assertEqual(rc, 0)
        self.assertEqual(steps, ["git clone -b main https://example.invalid/cluster-toolkit.git"])


class GoPathRunTests(SimpleTestCase):
    """Go's folders are added to /etc/bashrc once, however often the first
    boot runs"""

    def test_added_once(self):
        tmp = Path(tempfile.mkdtemp(prefix="ofe-startup-test-"))
        self.addCleanup(shutil.rmtree, tmp)
        bashrc = tmp / "bashrc"
        bashrc.write_text("# existing\n")
        script = tmp / "go-path.sh"
        script.write_text(go_path_section(STARTUP.read_text()).replace("/etc/bashrc", str(bashrc)))
        for _ in range(2):
            result = subprocess.run(
                ["bash", str(script)], capture_output=True, text=True, check=False, timeout=60
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(
            bashrc.read_text(), "# existing\nexport PATH=$PATH:/usr/local/go/bin:~/go/bin\n"
        )
