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

"""Unit and workflow-contract tests for hotfix release automation.

Run: python3 -m unittest tools/tests/test_hotfix_version_update.py -v
"""

import json
import os
import re
import shutil
import stat
import subprocess
import tempfile
import unittest

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))
SCRIPT_PATH = os.path.join(REPO_ROOT, "tools/hotfix-version-update.sh")
DRAFT_WF_PATH = os.path.join(REPO_ROOT, ".github/workflows/draft-release-notes.yml")
PUBLISH_WF_PATH = os.path.join(REPO_ROOT, ".github/workflows/publish-release-notes.yml")
HOTFIX_TF_PATH = os.path.join(REPO_ROOT, "tools/cloud-build/provision/hotfix-tests.tf")
PROVISION_README_PATH = os.path.join(REPO_ROOT, "tools/cloud-build/provision/README.md")


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


def _extract_workflow_release_script(workflow_path: str) -> str:
    """Extract the Get Release Information bash block from a workflow YAML using stdlib."""
    with open(workflow_path, "r", encoding="utf-8") as f:
        lines = f.readlines()

    in_release_step = False
    in_run_block = False
    run_indent = 0
    collected = []
    for line in lines:
        if "id: release" in line:
            in_release_step = True
            continue
        if in_release_step and not in_run_block:
            if re.match(r"^\s*run:\s*\|\s*$", line):
                in_run_block = True
                run_indent = len(line) - len(line.lstrip())
            continue
        if in_run_block:
            if line.strip() == "":
                collected.append("")
                continue
            indent = len(line) - len(line.lstrip())
            if indent <= run_indent:
                break
            collected.append(line[run_indent + 2 :].rstrip("\n"))
    if not collected:
        raise AssertionError(f"Could not extract release step run block from {workflow_path}")
    return "\n".join(collected) + "\n"


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

        # Initial main commit (v1.109.0) with both .go and versions.tf files
        os.makedirs(os.path.join(self.work_repo, "pkg"))
        os.makedirs(os.path.join(self.work_repo, "modules/example"))
        with open(os.path.join(self.work_repo, "pkg/version.go"), "w", encoding="utf-8") as f:
            f.write('package pkg\nconst Version = "v1.109.0"\n')
        with open(
            os.path.join(self.work_repo, "modules/example/versions.tf"),
            "w",
            encoding="utf-8",
        ) as f:
            f.write('module_name = "blueprints/terraform/hpc-toolkit:example/v1.109.0"\n')
        with open(os.path.join(self.work_repo, "README.md"), "w", encoding="utf-8") as f:
            f.write("Latest release: v1.109.0\n")
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
        self.fail_release_view = False
        self.open_prs = []
        self._sync_gh_state()

        # Mock git-sed (portable replacement for git-extras' git-sed)
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
    if os.path.exists(os.path.join(STATE_DIR, "fail_release_view")):
        sys.exit(1)
    with open(os.path.join(STATE_DIR, "latest_tag.txt"), "r", encoding="utf-8") as f:
        tag = f.read().strip()
    if "--jq" in args:
        print(tag)
    else:
        print(json.dumps({{"tagName": tag}}))
    sys.exit(0)

if args[:2] == ["release", "list"]:
    with open(os.path.join(STATE_DIR, "latest_tag.txt"), "r", encoding="utf-8") as f:
        tag = f.read().strip()
    if "--jq" in args:
        print(tag)
    else:
        print(json.dumps([{{"tagName": tag}}]))
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
        fail_marker = os.path.join(self.gh_state_dir, "fail_release_view")
        if self.fail_release_view:
            with open(fail_marker, "w", encoding="utf-8") as f:
                f.write("1")
        elif os.path.exists(fail_marker):
            os.remove(fail_marker)

    def _run_script(self, *args: str, extra_env: dict = None) -> subprocess.CompletedProcess:
        self._sync_gh_state()
        env = os.environ.copy()
        env["PATH"] = f"{self.bin_dir}:{env['PATH']}"
        if extra_env:
            env.update(extra_env)
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

    def test_rejects_invalid_repo_slug_and_malformed_release_tag(self) -> None:
        res_bad_repo = self._run_script("-r", "invalidrepo")
        self.assertNotEqual(res_bad_repo.returncode, 0)
        self.assertIn("must be in 'owner/repo' format", res_bad_repo.stderr)

        self.latest_tag = "v1.109"
        res_bad_tag = self._run_script()
        self.assertNotEqual(res_bad_tag.returncode, 0)
        self.assertIn("does not match vX.Y.Z", res_bad_tag.stderr)

    def test_rejects_branch_cut_from_develop_even_after_develop_advances(self) -> None:
        _git(self.work_repo, "switch", "-c", "hotfix-v1.109.1", "develop")
        _git(self.work_repo, "push", "-u", "origin", "hotfix-v1.109.1")

        # Advance develop further after hotfix was cut from older develop commit
        _git(self.work_repo, "switch", "develop")
        with open(os.path.join(self.work_repo, "pkg/feature2.go"), "w", encoding="utf-8") as f:
            f.write("package pkg\n// another sprint commit\n")
        _git(self.work_repo, "add", ".")
        _git(self.work_repo, "commit", "-m", "feat: second sprint commit")
        _git(self.work_repo, "push", "origin", "develop")

        res = self._run_script("-b", "hotfix-v1.109.1")
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("contains 1 commit(s) from 'develop' that are not on 'main'", res.stderr)

    def test_rejects_branch_behind_main(self) -> None:
        _git(self.work_repo, "switch", "-c", "hotfix-v1.109.1", "main")
        _git(self.work_repo, "push", "-u", "origin", "hotfix-v1.109.1")

        _git(self.work_repo, "switch", "main")
        with open(os.path.join(self.work_repo, "pkg/extra.go"), "w", encoding="utf-8") as f:
            f.write("package pkg\n")
        _git(self.work_repo, "add", ".")
        _git(self.work_repo, "commit", "-m", "main moved ahead")
        _git(self.work_repo, "push", "origin", "main")

        res = self._run_script("-b", "hotfix-v1.109.1")
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("is behind 'origin/main'", res.stderr)

    def test_auto_cuts_hotfix_updates_go_and_tf_files_and_warns_on_rc(self) -> None:
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

        main_sha = _git(self.work_repo, "rev-parse", "origin/main")
        _git(self.work_repo, "fetch", "origin")
        hotfix_sha = _git(self.work_repo, "rev-parse", "origin/hotfix-v1.109.1")
        self.assertEqual(main_sha, hotfix_sha)

        # Verify version/v1.109.1 updated both .go and versions.tf, but left README.md untouched
        go_text = _git(self.work_repo, "show", "origin/version/v1.109.1:pkg/version.go")
        tf_text = _git(
            self.work_repo, "show", "origin/version/v1.109.1:modules/example/versions.tf"
        )
        readme_text = _git(self.work_repo, "show", "origin/version/v1.109.1:README.md")
        self.assertIn("v1.109.1", go_text)
        self.assertIn("v1.109.1", tf_text)
        self.assertIn("v1.109.0", readme_text)

        prs = self._created_prs()
        self.assertEqual(len(prs), 1)
        self.assertEqual(prs[0]["args"][prs[0]["args"].index("--base") + 1], "hotfix-v1.109.1")
        self.assertEqual(prs[0]["args"][prs[0]["args"].index("--head") + 1], "version/v1.109.1")

        self.assertIn("WARNING: An in-flight release candidate PR is currently open", res.stdout)
        self.assertIn("release-candidate-v1.110.0", res.stdout)
        self.assertIn("HOTFIX-test-<test_name>", res.stdout)
        self.assertIn("tools/publish_release.sh", res.stdout)
        self.assertIn("--base develop --head main", res.stdout)
        self.assertNotIn("hpc-toolkit", res.stdout)
        self.assertNotIn("--all -c", res.stdout)

    def test_release_list_fallback_and_hotfix_repo_env_var(self) -> None:
        self.fail_release_view = True
        res = self._run_script(extra_env={"HOTFIX_REPO": "example-fork/cluster-toolkit"})
        self.assertEqual(res.returncode, 0, msg=res.stderr)

        prs = self._created_prs()
        self.assertEqual(len(prs), 1)
        self.assertIn("https://github.com/example-fork/cluster-toolkit/pull/", prs[0]["url"])

    def test_opens_draft_main_pr_and_is_idempotent_on_rerun(self) -> None:
        _git(self.work_repo, "switch", "-c", "hotfix-v1.109.1", "main")
        with open(os.path.join(self.work_repo, "pkg/fix.go"), "w", encoding="utf-8") as f:
            f.write("package pkg\n// fix crash\n")
        _git(self.work_repo, "add", ".")
        _git(self.work_repo, "commit", "-m", "fix: resolve crash")
        _git(self.work_repo, "push", "-u", "origin", "hotfix-v1.109.1")

        # First run creates version PR and draft main PR
        res1 = self._run_script("-b", "hotfix-v1.109.1")
        self.assertEqual(res1.returncode, 0, msg=res1.stderr)
        prs = self._created_prs()
        self.assertEqual(len(prs), 2)
        main_pr_args = prs[1]["args"]
        self.assertIn("--draft", main_pr_args)
        self.assertEqual(main_pr_args[main_pr_args.index("--base") + 1], "main")
        self.assertEqual(main_pr_args[main_pr_args.index("--head") + 1], "hotfix-v1.109.1")

        # Second run with both PRs already open should not create duplicate PRs
        self.open_prs = [
            {
                "number": 100,
                "title": "Update Toolkit release to v1.109.1",
                "headRefName": "version/v1.109.1",
                "baseRefName": "hotfix-v1.109.1",
                "url": prs[0]["url"],
            },
            {
                "number": 101,
                "title": "Hotfix release v1.109.1",
                "headRefName": "hotfix-v1.109.1",
                "baseRefName": "main",
                "url": prs[1]["url"],
            },
        ]
        res2 = self._run_script("-b", "hotfix-v1.109.1")
        self.assertEqual(res2.returncode, 0, msg=res2.stderr)
        self.assertEqual(len(self._created_prs()), 2)
        self.assertIn("Open version-bump PR already exists", res2.stdout)
        self.assertIn("Open hotfix PR to main already exists", res2.stdout)

    def test_refuses_to_overwrite_stale_version_branch_without_open_pr(self) -> None:
        _git(self.work_repo, "switch", "-c", "hotfix-v1.109.1", "main")
        _git(self.work_repo, "push", "-u", "origin", "hotfix-v1.109.1")
        _git(self.work_repo, "switch", "-c", "version/v1.109.1", "hotfix-v1.109.1")
        _git(self.work_repo, "push", "-u", "origin", "version/v1.109.1")

        res = self._run_script("-b", "hotfix-v1.109.1")
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Remote branch 'version/v1.109.1' already exists without an open PR", res.stderr)

    def test_custom_repo_flag_targets_fork(self) -> None:
        res = self._run_script("-r", "example-fork/cluster-toolkit")
        self.assertEqual(res.returncode, 0, msg=res.stderr)

        prs = self._created_prs()
        self.assertEqual(len(prs), 1)
        self.assertIn("https://github.com/example-fork/cluster-toolkit/pull/", prs[0]["url"])
        self.assertIn("gh pr create -R example-fork/cluster-toolkit --base develop --head main", res.stdout)


class ReleaseWorkflowContractTest(unittest.TestCase):
    def _run_release_step(self, workflow_path: str, branch: str) -> subprocess.CompletedProcess:
        script = _extract_workflow_release_script(workflow_path)
        gh_out = os.path.join(tempfile.gettempdir(), f"gh-out-{os.getpid()}.txt")
        self.addCleanup(lambda: os.path.exists(gh_out) and os.remove(gh_out))
        env = os.environ.copy()
        env["BRANCH"] = branch
        env["GITHUB_OUTPUT"] = gh_out
        res = subprocess.run(["bash", "-c", script], capture_output=True, text=True, env=env)
        if res.returncode == 0 and os.path.exists(gh_out):
            with open(gh_out, "r", encoding="utf-8") as f:
                res.stdout += f.read()
        return res

    def test_workflows_accept_valid_hotfix_and_rc_branches(self) -> None:
        for wf in (DRAFT_WF_PATH, PUBLISH_WF_PATH):
            for branch, expected in [
                ("hotfix-v1.109.1", "version=v1.109.1"),
                ("hotfix-1.109.1", "version=v1.109.1"),
                ("release-candidate-v1.110.0", "version=v1.110.0"),
            ]:
                res = self._run_release_step(wf, branch)
                self.assertEqual(res.returncode, 0, msg=f"{wf} failed on {branch}: {res.stderr}")
                self.assertIn(expected, res.stdout)

    def test_workflows_reject_non_semver_branches(self) -> None:
        for wf in (DRAFT_WF_PATH, PUBLISH_WF_PATH):
            for bad_branch in ("hotfix-gke-crash", "release-candidate-test", "feature/foo"):
                res = self._run_release_step(wf, bad_branch)
                self.assertNotEqual(res.returncode, 0, msg=f"{wf} unexpectedly accepted {bad_branch}")

    def test_hotfix_provision_trigger_and_hygiene(self) -> None:
        with open(HOTFIX_TF_PATH, "r", encoding="utf-8") as f:
            tf_text = f.read()
        self.assertIn('_TEST_PREFIX = "hotfix-"', tf_text)
        self.assertIn("approval_required = true", tf_text)
        self.assertIn('branch          = "^(main|hotfix-.*)$"', tf_text)
        self.assertIn('owner = "GoogleCloudPlatform"', tf_text)
        self.assertIn('name  = "cluster-toolkit"', tf_text)

        with open(PROVISION_README_PATH, "r", encoding="utf-8") as f:
            readme_text = f.read()
        self.assertIn("google_cloudbuild_trigger.hotfix_test", readme_text)

        with open(SCRIPT_PATH, "r", encoding="utf-8") as f:
            script_text = f.read()
        self.assertNotIn("hpc-toolkit", script_text)
        self.assertNotRegex(script_text, r"\b(go/|b/)\S+")


if __name__ == "__main__":
    unittest.main()
