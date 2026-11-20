#!/usr/bin/env python3
# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Unit tests for tools/hotfix-version-update.sh.

Run: python3 -m unittest tools/tests/test_hotfix_version_update.py -v
"""

import json
import os
import shutil
import stat
import subprocess
import tempfile
import unittest

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))
SCRIPT_PATH = os.path.join(REPO_ROOT, "tools/hotfix-version-update.sh")


def _write_executable(path: str, content: str) -> None:
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)
    os.chmod(path, os.stat(path).st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)


def _git(cwd: str, *args: str) -> str:
    res = subprocess.run(
        ["git", *args],
        cwd=cwd,
        check=True,
        capture_output=True,
        text=True,
    )
    return res.stdout.strip()


class HotfixVersionUpdateTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmpdir = tempfile.mkdtemp(prefix="hotfix-test-")
        self.addCleanup(lambda: shutil.rmtree(self.tmpdir, ignore_errors=True))

        self.bare_repo = os.path.join(self.tmpdir, "remote.git")
        self.work_repo = os.path.join(self.tmpdir, "work")
        self.bin_dir = os.path.join(self.tmpdir, "bin")
        self.gh_state_dir = os.path.join(self.tmpdir, "gh_state")
        os.makedirs(self.bin_dir)
        os.makedirs(self.gh_state_dir)

        _git(self.tmpdir, "init", "--bare", "--initial-branch=main", self.bare_repo)
        _git(self.tmpdir, "clone", self.bare_repo, self.work_repo)
        _git(self.work_repo, "config", "user.email", "test@example.com")
        _git(self.work_repo, "config", "user.name", "Test User")

        # Initial main commit (v1.109.0)
        os.makedirs(os.path.join(self.work_repo, "pkg"))
        with open(os.path.join(self.work_repo, "pkg/version.go"), "w", encoding="utf-8") as f:
            f.write('package pkg\nconst Version = "v1.109.0"\n')
        _git(self.work_repo, "add", ".")
        _git(self.work_repo, "commit", "-m", "Release v1.109.0")
        _git(self.work_repo, "push", "-u", "origin", "main")

        # Create develop branch ahead of main with sprint commits
        _git(self.work_repo, "switch", "-c", "develop", "main")
        with open(os.path.join(self.work_repo, "pkg/feature.go"), "w", encoding="utf-8") as f:
            f.write("package pkg\n// sprint feature\n")
        _git(self.work_repo, "add", ".")
        _git(self.work_repo, "commit", "-m", "feat: sprint work on develop")
        _git(self.work_repo, "push", "-u", "origin", "develop")

        # Mock state defaults
        self.latest_tag = "v1.109.0"
        self.open_prs = []
        self._sync_gh_state()

        # Mock git-sed (simple portable replacement for git-extras' git-sed)
        _write_executable(
            os.path.join(self.bin_dir, "git-sed"),
            """#!/usr/bin/env python3
import re
import sys

args = [a for a in sys.argv[1:] if a != "--"]
pattern, replacement = args[0], args[1]
files = args[2:]
for path in files:
    with open(path, "r", encoding="utf-8") as f:
        text = f.read()
    updated = re.sub(pattern, replacement, text)
    with open(path, "w", encoding="utf-8") as f:
        f.write(updated)
""",
        )

        # Mock gh CLI backed by local bare_repo and gh_state_dir
        _write_executable(
            os.path.join(self.bin_dir, "gh"),
            f"""#!/usr/bin/env python3
import json
import os
import subprocess
import sys

STATE_DIR = {self.gh_state_dir!r}
BARE_REPO = {self.bare_repo!r}
LOG_FILE = os.path.join(STATE_DIR, "gh_calls.log")

with open(LOG_FILE, "a", encoding="utf-8") as lf:
    lf.write(" ".join(sys.argv[1:]) + "\\n")

args = sys.argv[1:]
if not args:
    sys.exit(1)

if args[:2] == ["auth", "status"]:
    sys.exit(0)

if args[:2] == ["release", "view"]:
    with open(os.path.join(STATE_DIR, "latest_tag.txt"), "r", encoding="utf-8") as f:
        tag = f.read().strip()
    if "--jq" in args:
        print(tag)
    else:
        print(json.dumps({{"tagName": tag}}))
    sys.exit(0)

if args[:2] == ["pr", "list"]:
    with open(os.path.join(STATE_DIR, "open_prs.json"), "r", encoding="utf-8") as f:
        prs = json.load(f)
    if "--base" in args:
        base = args[args.index("--base") + 1]
        prs = [p for p in prs if p.get("baseRefName") == base]
    if "--head" in args:
        head = args[args.index("--head") + 1]
        prs = [p for p in prs if p.get("headRefName") == head]
    jq_expr = args[args.index("--jq") + 1] if "--jq" in args else ""
    if "startswith" in jq_expr:
        filtered = [p for p in prs if p.get("headRefName", "").startswith("release-candidate-")]
        print(json.dumps(filtered))
    elif ".[0].url" in jq_expr:
        print(prs[0]["url"] if prs else "")
    else:
        print(json.dumps(prs))
    sys.exit(0)

if args[:2] == ["pr", "create"]:
    counter_file = os.path.join(STATE_DIR, "pr_counter.txt")
    num = 100
    if os.path.exists(counter_file):
        with open(counter_file, "r", encoding="utf-8") as f:
            num = int(f.read().strip()) + 1
    with open(counter_file, "w", encoding="utf-8") as f:
        f.write(str(num))
    repo = args[args.index("-R") + 1] if "-R" in args else "GoogleCloudPlatform/cluster-toolkit"
    url = f"https://github.com/{{repo}}/pull/{{num}}"
    with open(os.path.join(STATE_DIR, "created_prs.jsonl"), "a", encoding="utf-8") as cf:
        cf.write(json.dumps({{"args": args, "url": url}}) + "\\n")
    print(url)
    sys.exit(0)

if args[:2] == ["repo", "clone"]:
    dest = args[3]
    extra = args[5:] if "--" in args else []
    subprocess.run(["git", "clone", *extra, BARE_REPO, dest], check=True)
    subprocess.run(["git", "-C", dest, "config", "user.email", "test@example.com"], check=True)
    subprocess.run(["git", "-C", dest, "config", "user.name", "Test User"], check=True)
    sys.exit(0)

sys.stderr.write(f"Unhandled mock gh args: {{args}}\\n")
sys.exit(1)
""",
        )

    def _sync_gh_state(self) -> None:
        with open(os.path.join(self.gh_state_dir, "latest_tag.txt"), "w", encoding="utf-8") as f:
            f.write(self.latest_tag)
        with open(os.path.join(self.gh_state_dir, "open_prs.json"), "w", encoding="utf-8") as f:
            json.dump(self.open_prs, f)

    def _run_script(self, *args: str) -> subprocess.CompletedProcess:
        self._sync_gh_state()
        env = os.environ.copy()
        env["PATH"] = f"{self.bin_dir}:{env['PATH']}"
        return subprocess.run(
            ["bash", SCRIPT_PATH, *args],
            capture_output=True,
            text=True,
            env=env,
        )

    def _created_prs(self) -> list:
        path = os.path.join(self.gh_state_dir, "created_prs.jsonl")
        if not os.path.exists(path):
            return []
        with open(path, "r", encoding="utf-8") as f:
            return [json.loads(line) for line in f if line.strip()]

    def test_rejects_non_semver_branch_name(self) -> None:
        res = self._run_script("-b", "hotfix-gke-crash")
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("must match ^hotfix-v", res.stderr)

    def test_rejects_mismatched_patch_version(self) -> None:
        res = self._run_script("-b", "hotfix-v1.109.2")
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("does not match the next patch release 'hotfix-v1.109.1'", res.stderr)

    def test_rejects_branch_cut_from_develop(self) -> None:
        _git(self.work_repo, "switch", "-c", "hotfix-v1.109.1", "develop")
        _git(self.work_repo, "push", "-u", "origin", "hotfix-v1.109.1")

        res = self._run_script("-b", "hotfix-v1.109.1")
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("contains 1 commit(s) from 'develop' that are not on 'main'", res.stderr)

    def test_rejects_branch_behind_main(self) -> None:
        _git(self.work_repo, "switch", "-c", "hotfix-v1.109.1", "main")
        _git(self.work_repo, "push", "-u", "origin", "hotfix-v1.109.1")

        # Advance main
        _git(self.work_repo, "switch", "main")
        with open(os.path.join(self.work_repo, "pkg/extra.go"), "w", encoding="utf-8") as f:
            f.write("package pkg\n")
        _git(self.work_repo, "add", ".")
        _git(self.work_repo, "commit", "-m", "main moved ahead")
        _git(self.work_repo, "push", "origin", "main")

        res = self._run_script("-b", "hotfix-v1.109.1")
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("is behind 'origin/main'", res.stderr)

    def test_auto_cuts_hotfix_from_main_and_warns_on_open_rc(self) -> None:
        self.open_prs = [
            {
                "number": 999,
                "title": "Release candidate v1.110.0",
                "headRefName": "release-candidate-v1.110.0",
                "baseRefName": "main",
                "url": "https://github.com/GoogleCloudPlatform/cluster-toolkit/pull/999",
            }
        ]
        res = self._run_script()
        self.assertEqual(res.returncode, 0, msg=res.stderr)

        # Verify branch cut directly from main and version-bump PR created
        main_sha = _git(self.work_repo, "rev-parse", "origin/main")
        _git(self.work_repo, "fetch", "origin")
        hotfix_sha = _git(self.work_repo, "rev-parse", "origin/hotfix-v1.109.1")
        self.assertEqual(main_sha, hotfix_sha)

        prs = self._created_prs()
        self.assertEqual(len(prs), 1)
        self.assertIn("--base", prs[0]["args"])
        self.assertEqual(prs[0]["args"][prs[0]["args"].index("--base") + 1], "hotfix-v1.109.1")
        self.assertEqual(prs[0]["args"][prs[0]["args"].index("--head") + 1], "version/v1.109.1")

        # Verify RC warning, targeted test hint, OSS exit gate, back-merge, and no legacy hpc-toolkit/--all
        self.assertIn("WARNING: An in-flight release candidate PR is currently open", res.stdout)
        self.assertIn("release-candidate-v1.110.0", res.stdout)
        self.assertIn("HOTFIX-test-<test_name>", res.stdout)
        self.assertIn("tools/publish_release.sh", res.stdout)
        self.assertIn("--base develop --head main", res.stdout)
        self.assertNotIn("hpc-toolkit", res.stdout)
        self.assertNotIn("--all -c", res.stdout)

    def test_opens_draft_main_pr_when_hotfix_branch_has_commits_from_main(self) -> None:
        _git(self.work_repo, "switch", "-c", "hotfix-v1.109.1", "main")
        with open(os.path.join(self.work_repo, "pkg/fix.go"), "w", encoding="utf-8") as f:
            f.write("package pkg\n// fix crash\n")
        _git(self.work_repo, "add", ".")
        _git(self.work_repo, "commit", "-m", "fix: resolve crash")
        _git(self.work_repo, "push", "-u", "origin", "hotfix-v1.109.1")

        res = self._run_script("-b", "hotfix-v1.109.1")
        self.assertEqual(res.returncode, 0, msg=res.stderr)

        prs = self._created_prs()
        self.assertEqual(len(prs), 2)
        # Second PR should be the draft PR from hotfix-v1.109.1 -> main
        main_pr_args = prs[1]["args"]
        self.assertIn("--draft", main_pr_args)
        self.assertEqual(main_pr_args[main_pr_args.index("--base") + 1], "main")
        self.assertEqual(main_pr_args[main_pr_args.index("--head") + 1], "hotfix-v1.109.1")


if __name__ == "__main__":
    unittest.main()
