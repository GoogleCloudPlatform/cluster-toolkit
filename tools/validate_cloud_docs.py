#!/usr/bin/env python3
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
"""Validates public Google Cloud Documentation (cloud.google.com) against the repository.

Modes:
  1. --check-pr:
     Inspects changed files in a PR (via git diff or --changed-files) and prints
     a reminder table listing any public cloud.google.com pages that document
     the modified blueprints or CLI commands. Exits 1 when --fail-on-impact is
     set and Cloud-Doc-backed files are modified (exits 0 otherwise).

  2. --check-live (or --local-dir <dir>):
     Fetches the live public https://cloud.google.com/cluster-toolkit/docs/...
     pages (or reads local HTML/Markdown files from --local-dir) and verifies
     that documented storage modules, deployment variables, required GCP APIs,
     CLI subcommands, and GitHub file links match the current repository.
     Exits 1 when drift is detected so weekly/release checks surface needed updates.
"""

from __future__ import annotations

import argparse
import html as html_lib
import http.client
import os
import re
import subprocess
import sys
import time
from typing import Callable
import urllib.error
import urllib.request
import yaml

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
CLOUD_DOCS_BASE = "https://cloud.google.com"
USER_AGENT = "cluster-toolkit-cloud-docs-validator/1.0"

# Mapping from repository path prefixes/patterns to public Cloud Docs slugs.
PR_PATH_TO_CLOUD_DOCS: list[tuple[str, list[str], str]] = [
    (
        "examples/machine-learning/a3-ultragpu-8g/",
        ["/cluster-toolkit/docs/deploy/slurm/create-a3-ultra-cluster"],
        "A3 Ultra Slurm blueprint or deployment variables changed",
    ),
    (
        "examples/machine-learning/a4-highgpu-8g/",
        ["/cluster-toolkit/docs/deploy/slurm/create-a4-cluster"],
        "A4 High Slurm blueprint or deployment variables changed",
    ),
    (
        "examples/machine-learning/a4x-highgpu-4g/",
        ["/cluster-toolkit/docs/deploy/slurm/create-a4x-cluster"],
        "A4X High Slurm blueprint or deployment variables changed",
    ),
    (
        "examples/machine-learning/a4x-maxgpu-4g-metal/",
        ["/cluster-toolkit/docs/deploy/slurm/create-a4x-max-cluster"],
        "A4X Max Slurm blueprint or deployment variables changed",
    ),
    (
        "examples/gke-a3-ultragpu/",
        [
            "/cluster-toolkit/docs/deploy/gke/create-a3-ultra-cluster",
            "/ai-hypercomputer/docs/create/gke-ai-hypercompute",
        ],
        "GKE A3 Ultra blueprint or deployment template changed",
    ),
    (
        "examples/gke-a4/",
        [
            "/cluster-toolkit/docs/deploy/gke/create-a4-cluster",
            "/ai-hypercomputer/docs/create/gke-ai-hypercompute",
        ],
        "GKE A4 blueprint or deployment template changed",
    ),
    (
        "examples/gke-a4x/",
        [
            "/cluster-toolkit/docs/deploy/gke/create-a4x-cluster",
            "/ai-hypercomputer/docs/create/gke-ai-hypercompute",
        ],
        "GKE A4X blueprint or deployment template changed",
    ),
    (
        "examples/hpc-slurm.yaml",
        ["/cluster-toolkit/docs/quickstarts/slurm-cluster"],
        "Quickstart Slurm blueprint changed",
    ),
    (
        "examples/README.md",
        ["/cluster-toolkit/docs/setup/cluster-blueprint-catalog"],
        "Core blueprint catalog table changed",
    ),
    (
        "community/examples/README.md",
        ["/cluster-toolkit/docs/setup/cluster-blueprint-catalog"],
        "Community blueprint catalog table changed",
    ),
    (
        "cmd/job/",
        ["/cluster-toolkit/docs/reference/gcluster-job"],
        "gcluster job CLI commands or flags changed",
    ),
    (
        "cmd/cluster/",
        ["/cluster-toolkit/docs/reference/gcluster-cluster"],
        "gcluster cluster CLI commands or flags changed",
    ),
    (
        "cmd/",
        ["/cluster-toolkit/docs/reference/gcluster"],
        "Top-level gcluster CLI commands or flags changed",
    ),
]

SLURM_GPU_PAGES: list[tuple[str, str]] = [
    (
        "/cluster-toolkit/docs/deploy/slurm/create-a3-ultra-cluster",
        "examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml",
    ),
    (
        "/cluster-toolkit/docs/deploy/slurm/create-a4-cluster",
        "examples/machine-learning/a4-highgpu-8g/a4high-slurm-blueprint.yaml",
    ),
    (
        "/cluster-toolkit/docs/deploy/slurm/create-a4x-cluster",
        "examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-blueprint.yaml",
    ),
    (
        "/cluster-toolkit/docs/deploy/slurm/create-a4x-max-cluster",
        "examples/machine-learning/a4x-maxgpu-4g-metal/a4xmax-bm-slurm-blueprint.yaml",
    ),
]

GKE_GPU_PAGES: list[tuple[str, str, str]] = [
    (
        "/cluster-toolkit/docs/deploy/gke/create-a3-ultra-cluster",
        "examples/gke-a3-ultragpu/gke-a3-ultragpu.yaml",
        "examples/gke-a3-ultragpu/gke-a3-ultragpu-deployment.yaml",
    ),
    (
        "/cluster-toolkit/docs/deploy/gke/create-a4-cluster",
        "examples/gke-a4/gke-a4.yaml",
        "examples/gke-a4/gke-a4-deployment.yaml",
    ),
    (
        "/cluster-toolkit/docs/deploy/gke/create-a4x-cluster",
        "examples/gke-a4x/gke-a4x.yaml",
        "examples/gke-a4x/gke-a4x-deployment.yaml",
    ),
]

ALL_MONITORED_PAGES: list[str] = [
    "/cluster-toolkit/docs/overview",
    "/cluster-toolkit/docs/setup/configure-environment",
    "/cluster-toolkit/docs/setup/cluster-blueprint-catalog",
    "/cluster-toolkit/docs/quickstarts/slurm-cluster",
    "/cluster-toolkit/docs/deploy/slurm/create-a3-ultra-cluster",
    "/cluster-toolkit/docs/deploy/slurm/create-a4-cluster",
    "/cluster-toolkit/docs/deploy/slurm/create-a4x-cluster",
    "/cluster-toolkit/docs/deploy/slurm/create-a4x-max-cluster",
    "/cluster-toolkit/docs/deploy/gke/create-a3-ultra-cluster",
    "/cluster-toolkit/docs/deploy/gke/create-a4-cluster",
    "/cluster-toolkit/docs/deploy/gke/create-a4x-cluster",
    "/cluster-toolkit/docs/reference/gcluster",
    "/cluster-toolkit/docs/reference/gcluster-job",
    "/cluster-toolkit/docs/reference/gcluster-cluster",
    "/ai-hypercomputer/docs/create/gke-ai-hypercompute",
]

INTERNAL_CMD_NAMES = {
    "gcluster",
    "show-id",
    "test",
    "test-command",
    "dummy-success",
    "dummy-error",
    "dummy-exit-error",
    "custom",
    "set",
    "show",
}


def extract_article_html(raw_html: str) -> str:
    """Extracts the <article> body from a DevSite HTML page (or returns full input)."""
    match = re.search(
        r"<article[^>]*>(.*?)</article>", raw_html, re.DOTALL | re.IGNORECASE
    )
    return match.group(1) if match else raw_html


def html_to_text(raw_html: str) -> str:
    """Converts HTML/Markdown article content to normalized plain text."""
    article = extract_article_html(raw_html)
    without_tags = re.sub(r"<[^>]+>", " ", article)
    unescaped = html_lib.unescape(without_tags)
    return re.sub(r"\s+", " ", unescaped).strip()


def fetch_page(
    slug: str,
    local_dir: str | None = None,
    timeout: int = 15,
    retries: int = 3,
) -> str:
    """Fetches a Cloud Docs page via HTTPS or reads it from local_dir if specified."""
    if local_dir:
        rel = slug.lstrip("/")
        for candidate in (
            os.path.join(local_dir, rel),
            os.path.join(local_dir, f"{rel}.html"),
            os.path.join(local_dir, f"{rel}.md"),
        ):
            if os.path.isfile(candidate):
                with open(candidate, encoding="utf-8") as f:
                    return f.read()
        raise FileNotFoundError(f"No local doc file found for {slug} in {local_dir}")

    url = f"{CLOUD_DOCS_BASE}{slug}"
    last_err: Exception | None = None
    for attempt in range(retries):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                return resp.read().decode("utf-8", errors="replace")
        except (
            urllib.error.URLError,
            OSError,
            http.client.HTTPException,
            ValueError,
        ) as exc:
            last_err = exc
            if isinstance(exc, urllib.error.HTTPError) and exc.code not in (
                429,
                500,
                502,
                503,
                504,
            ):
                break
            if attempt + 1 < retries:
                time.sleep(0.5 * (2**attempt))
    raise RuntimeError(f"Failed to fetch {url}: {last_err}")


def get_blueprint_module_sources(
    blueprint_rel_path: str, repo_root: str = REPO_ROOT
) -> set[str]:
    """Returns the set of module source paths used in a blueprint YAML file."""
    full_path = os.path.join(repo_root, blueprint_rel_path)
    with open(full_path, encoding="utf-8") as f:
        data = yaml.safe_load(f)
    sources: set[str] = set()
    if not isinstance(data, dict):
        return sources
    for group in data.get("deployment_groups", []) or []:
        if not isinstance(group, dict):
            continue
        for mod in group.get("modules", []) or []:
            if isinstance(mod, dict) and mod.get("source"):
                sources.add(str(mod["source"]).strip())
    return sources


def extract_cli_subcommands(
    cmd_rel_dir: str, repo_root: str = REPO_ROOT
) -> set[str]:
    """Extracts user-facing CLI subcommand names from Use: fields in a cmd directory."""
    full_dir = os.path.join(repo_root, cmd_rel_dir)
    commands: set[str] = set()
    if not os.path.isdir(full_dir):
        return commands
    for entry in sorted(os.listdir(full_dir)):
        if not entry.endswith(".go") or entry.endswith("_test.go"):
            continue
        path = os.path.join(full_dir, entry)
        with open(path, encoding="utf-8") as f:
            content = f.read()
        for match in re.finditer(r'\bUse:\s*"([^"]+)"', content):
            parts = match.group(1).strip().split()
            if parts:
                sub = parts[0]
                if sub not in INTERNAL_CMD_NAMES:
                    commands.add(sub)
    return commands


def check_github_blob_links(
    slug: str, article_html: str, repo_root: str = REPO_ROOT
) -> list[str]:
    """Checks that any GitHub blob/tree links to cluster-toolkit resolve in repo_root."""
    errors: list[str] = []
    pattern = re.compile(
        r"https://github\.com/GoogleCloudPlatform/(?:cluster-toolkit|hpc-toolkit)/(?:blob|tree)/[^/]+/([^\s\"'<>#?]+)"
    )
    seen: set[str] = set()
    for match in pattern.finditer(article_html):
        rel_path = match.group(1).rstrip("/.,;:)")
        if not rel_path or rel_path in seen:
            continue
        seen.add(rel_path)
        if not os.path.exists(os.path.join(repo_root, rel_path)):
            errors.append(
                f"{slug}: broken GitHub repo path '{rel_path}' (does not exist on current branch)"
            )
    return errors


def check_code_block_paths(
    slug: str, article_html: str, repo_root: str = REPO_ROOT
) -> list[str]:
    """Checks that any examples/*.yaml or community/examples/*.yaml paths in <pre> blocks exist."""
    errors: list[str] = []
    pre_blocks = [
        html_lib.unescape(re.sub(r"<[^>]+>", "", p))
        for p in re.findall(r"<pre[^>]*>(.*?)</pre>", article_html, re.DOTALL | re.IGNORECASE)
    ]
    seen_paths: set[str] = set()
    pattern = re.compile(
        r"(?<![a-zA-Z0-9_./-])((?:community/)?examples/[a-zA-Z0-9_./-]+\.ya?ml)\b"
    )
    for pre_text in pre_blocks:
        for match in pattern.finditer(pre_text):
            rel_ex = match.group(1)
            if rel_ex not in seen_paths:
                seen_paths.add(rel_ex)
                if not os.path.exists(os.path.join(repo_root, rel_ex)):
                    errors.append(
                        f"{slug}: code block references non-existent repo file '{rel_ex}'"
                    )
    return errors


def check_slurm_gpu_page(
    slug: str,
    blueprint_rel: str,
    raw_html: str,
    repo_root: str = REPO_ROOT,
) -> list[str]:
    """Validates a Slurm GPU deployment guide against its repository blueprint."""
    errors: list[str] = []
    text = html_to_text(raw_html)
    sources = get_blueprint_module_sources(blueprint_rel, repo_root=repo_root)
    uses_lustre = any("managed-lustre" in s for s in sources)
    uses_filestore = any("filestore" in s for s in sources)

    if uses_lustre and not uses_filestore:
        if "HIGH_SCALE_SSD" in text:
            errors.append(
                f"{slug}: instructs users to configure Filestore (HIGH_SCALE_SSD), "
                f"but {blueprint_rel} uses Managed Lustre (modules/file-system/managed-lustre)"
            )
        if "Managed Lustre" not in text and "lustre.googleapis.com" not in text:
            errors.append(
                f"{slug}: missing Managed Lustre quota/API guidance required by {blueprint_rel}"
            )

    if "v1.62.0" in text:
        errors.append(
            f"{slug}: pins outdated Cluster Toolkit version 'v1.62.0' (update to current release tag)"
        )
    return errors


def check_gke_gpu_page(
    slug: str,
    blueprint_rel: str,
    deployment_rel: str,
    raw_html: str,
    repo_root: str = REPO_ROOT,
) -> list[str]:
    """Validates a GKE GPU deployment guide against its blueprint and deployment YAML."""
    errors: list[str] = []
    text = html_to_text(raw_html)
    bp_path = os.path.join(repo_root, blueprint_rel)
    dep_path = os.path.join(repo_root, deployment_rel)
    with open(bp_path, encoding="utf-8") as f:
        bp_text = f.read()
    with open(dep_path, encoding="utf-8") as f:
        dep_text = f.read()

    uses_reservation_affinity = (
        "reservation_affinity" in bp_text or "reservation_affinity" in dep_text
    )
    uses_flat_reservation = bool(
        re.search(r"\$\(vars\.reservation\)", bp_text)
        or re.search(r"^\s*reservation\s*:", dep_text, re.MULTILINE)
    )

    if uses_reservation_affinity and not uses_flat_reservation:
        if "reservation_affinity" not in text or re.search(
            r"For the\s+reservation\s+field,", text
        ):
            errors.append(
                f"{slug}: instructs users to set flat 'reservation' field, "
                f"but {blueprint_rel} requires nested 'reservation_affinity'"
            )
    elif uses_flat_reservation and not uses_reservation_affinity:
        if "reservation_affinity" in text:
            errors.append(
                f"{slug}: instructs users to set 'reservation_affinity', "
                f"but {blueprint_rel} uses flat 'reservation'"
            )

    article = extract_article_html(raw_html)
    pre_blocks = [
        html_lib.unescape(re.sub(r"<[^>]+>", "", p))
        for p in re.findall(r"<pre[^>]*>(.*?)</pre>", article, re.DOTALL | re.IGNORECASE)
    ]

    if any(re.search(r"gcluster\s+destroy\s+CLUSTER_NAME\b", p) for p in pre_blocks):
        errors.append(
            f"{slug}: runs 'gcluster destroy CLUSTER_NAME' (should pass deployment directory, not GKE cluster name)"
        )

    errors.extend(check_code_block_paths(slug, article, repo_root=repo_root))
    return errors


def check_configure_environment_page(slug: str, raw_html: str) -> list[str]:
    """Validates setup/configure-environment required GCP APIs."""
    errors: list[str] = []
    text = html_to_text(raw_html)
    for required_api in ("lustre.googleapis.com", "servicenetworking.googleapis.com"):
        if required_api not in text:
            errors.append(
                f"{slug}: missing required API '{required_api}' in gcloud services enable instructions"
            )
    return errors


def check_cli_reference_page(
    slug: str, cmd_rel_dir: str, raw_html: str, repo_root: str = REPO_ROOT
) -> list[str]:
    """Validates a CLI reference page against the subcommands defined in cmd_rel_dir."""
    errors: list[str] = []
    text = html_to_text(raw_html)
    expected_cmds = extract_cli_subcommands(cmd_rel_dir, repo_root=repo_root)
    for sub in sorted(expected_cmds):
        if not re.search(rf"\b{re.escape(sub)}\b", text):
            errors.append(
                f"{slug}: CLI subcommand '{sub}' (from {cmd_rel_dir}/) is not documented on page"
            )
    return errors


def check_blueprint_catalog_page(slug: str, raw_html: str) -> list[str]:
    """Validates setup/cluster-blueprint-catalog for removed OS images and obsolete footnotes."""
    errors: list[str] = []
    text = html_to_text(raw_html)
    if "hpc-centos-7" in text:
        errors.append(f"{slug}: references removed OS image 'hpc-centos-7'")
    if "DAOS drivers pre-installed" in text:
        errors.append(f"{slug}: contains obsolete DAOS drivers footnote")
    return errors


def validate_live_cloud_docs(
    fetcher: Callable[[str], str] | None = None,
    repo_root: str = REPO_ROOT,
) -> list[str]:
    """Runs all deterministic drift validations across the monitored Cloud Docs pages."""
    if fetcher is None:
        fetcher = fetch_page

    errors: list[str] = []
    cache: dict[str, str | None] = {}

    def get_page(slug: str) -> str | None:
        if slug in cache:
            return cache[slug]
        try:
            cache[slug] = fetcher(slug)
        except Exception as exc:  # pylint: disable=broad-except
            errors.append(f"{slug}: failed to load page: {exc}")
            cache[slug] = None
        return cache[slug]

    def run_check(slug: str, check: Callable[..., list[str]], *args, **kwargs) -> None:
        try:
            errors.extend(check(*args, **kwargs))
        except (OSError, yaml.YAMLError) as exc:
            errors.append(
                f"{slug}: {check.__name__} failed (stale repo mapping?): {exc}"
            )

    gke_slugs = {s for s, _, _ in GKE_GPU_PAGES}
    for slug in ALL_MONITORED_PAGES:
        raw = get_page(slug)
        if raw is not None:
            article = extract_article_html(raw)
            errors.extend(check_github_blob_links(slug, article, repo_root=repo_root))
            if slug not in gke_slugs:
                errors.extend(
                    check_code_block_paths(slug, article, repo_root=repo_root)
                )

    for slug, bp_rel in SLURM_GPU_PAGES:
        raw = get_page(slug)
        if raw is not None:
            run_check(
                slug, check_slurm_gpu_page, slug, bp_rel, raw, repo_root=repo_root
            )

    for slug, bp_rel, dep_rel in GKE_GPU_PAGES:
        raw = get_page(slug)
        if raw is not None:
            run_check(
                slug,
                check_gke_gpu_page,
                slug,
                bp_rel,
                dep_rel,
                raw,
                repo_root=repo_root,
            )

    env_raw = get_page("/cluster-toolkit/docs/setup/configure-environment")
    if env_raw is not None:
        errors.extend(
            check_configure_environment_page(
                "/cluster-toolkit/docs/setup/configure-environment", env_raw
            )
        )

    for slug, cmd_dir in (
        ("/cluster-toolkit/docs/reference/gcluster", "cmd"),
        ("/cluster-toolkit/docs/reference/gcluster-job", "cmd/job"),
        ("/cluster-toolkit/docs/reference/gcluster-cluster", "cmd/cluster"),
    ):
        raw = get_page(slug)
        if raw is not None:
            run_check(
                slug, check_cli_reference_page, slug, cmd_dir, raw, repo_root=repo_root
            )

    cat_raw = get_page("/cluster-toolkit/docs/setup/cluster-blueprint-catalog")
    if cat_raw is not None:
        errors.extend(
            check_blueprint_catalog_page(
                "/cluster-toolkit/docs/setup/cluster-blueprint-catalog", cat_raw
            )
        )

    return errors


def get_changed_files_from_git(
    base_ref: str = "origin/develop", repo_root: str = REPO_ROOT
) -> list[str]:
    """Returns changed file paths between base_ref and HEAD."""
    res = subprocess.run(
        ["git", "diff", "--name-only", "--no-renames", f"{base_ref}...HEAD"],
        cwd=repo_root,
        capture_output=True,
        text=True,
        check=False,
    )
    if res.returncode != 0:
        print(
            f"Warning: '{base_ref}...HEAD' failed ({res.stderr.strip()}); "
            "falling back to HEAD~1 (last commit only)",
            file=sys.stderr,
        )
        res = subprocess.run(
            ["git", "diff", "--name-only", "--no-renames", "HEAD~1", "HEAD"],
            cwd=repo_root,
            capture_output=True,
            text=True,
            check=False,
        )
    if res.returncode != 0:
        print(
            f"Warning: git diff failed (base_ref={base_ref}): {res.stderr.strip()}",
            file=sys.stderr,
        )
        return []
    return [line.strip() for line in res.stdout.splitlines() if line.strip()]


def match_pr_cloud_docs_impacts(
    changed_files: list[str],
) -> list[tuple[str, list[str], str]]:
    """Maps changed repo files to impacted public Cloud Docs URLs and reasons."""
    impacts: list[tuple[str, list[str], str]] = []
    for path in sorted(set(changed_files)):
        if path.endswith("_test.go"):
            continue
        for prefix, slugs, reason in PR_PATH_TO_CLOUD_DOCS:
            if path == prefix or (prefix.endswith("/") and path.startswith(prefix)):
                # For top-level cmd/, only match direct files in cmd/ (not subdirs already matched)
                if prefix == "cmd/" and "/" in path[len("cmd/") :]:
                    continue
                urls = [f"{CLOUD_DOCS_BASE}{s}" for s in slugs]
                impacts.append((path, urls, reason))
                break
    return impacts


def extract_cloud_docs_ack(pr_body: str | None) -> str | None:
    """Extracts a 'Cloud-Docs: <plan or none>' acknowledgment line from a PR description."""
    if not pr_body:
        return None
    match = re.search(
        r"^\s*(?:\*\*|`)?cloud[-_\s]?docs(?:\*\*|`)?\s*:\s*(\S.*)$",
        pr_body,
        re.IGNORECASE | re.MULTILINE,
    )
    return match.group(1).strip() if match else None


def format_pr_reminder_markdown(
    impacts: list[tuple[str, list[str], str]],
    ack: str | None = None,
) -> str:
    """Formats a Markdown reminder table for PR authors when Cloud-Doc-backed files change."""
    if not impacts:
        return (
            "### Cloud Docs Drift Check (`--check-pr`)\n\n"
            "No Cloud-Doc-backed blueprints or CLI commands were modified in this PR.\n"
        )
    lines = [
        "### Cloud Docs Reminder (`--check-pr`)",
        "",
        "This PR modifies blueprints or CLI commands that are documented on `cloud.google.com`. "
        "Please verify whether the corresponding public documentation pages also need an update:",
        "",
        "| Modified Repo File | Affected Public Cloud Docs URL(s) | Why |",
        "| :--- | :--- | :--- |",
    ]
    for path, urls, reason in impacts:
        url_links = "<br>".join(f"[{u}]({u})" for u in urls)
        lines.append(f"| `{path}` | {url_links} | {reason} |")
    lines.append("")
    if ack:
        lines.append(
            f"**Cloud-Docs Acknowledgment:** `{ack}` (acknowledged in PR description — check passed)"
        )
    else:
        lines.append(
            "**How to pass this check:** Add a `Cloud-Docs:` line to your PR description "
            "(for example, `Cloud-Docs: b/123456` or `Cloud-Docs: none (no public doc impact)`) "
            "to acknowledge and turn this check green."
        )
    lines.append("")
    return "\n".join(lines)


def _emit_github_annotations(impacts: list[tuple[str, list[str], str]]) -> None:
    """Emits ::warning annotations so impacts show inline on the PR diff."""
    if os.environ.get("GITHUB_ACTIONS") != "true":
        return
    for path, urls, reason in impacts:
        msg = (
            f"{reason}. Verify: {' '.join(urls)} "
            "(Add 'Cloud-Docs: <bug or none>' to PR description to pass check)"
        )
        msg = msg.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")
        prop = path.replace("%", "%25").replace(":", "%3A").replace(",", "%2C")
        print(f"::warning file={prop},title=Cloud Docs may need an update::{msg}")


def format_live_drift_markdown(errors: list[str]) -> str:
    """Formats the live Cloud Docs drift report as Markdown."""
    if not errors:
        return (
            "### Live Cloud Docs Drift Check (`--check-live`)\n\n"
            "All monitored `cloud.google.com/cluster-toolkit/docs/*` pages match the current branch.\n"
        )
    lines = [
        "### Live Cloud Docs Drift Detected (`--check-live`)",
        "",
        f"Found **{len(errors)}** drift item(s) between live `cloud.google.com` pages and the repository:",
        "",
    ]
    for err in errors:
        lines.append(
            f"- `{CLOUD_DOCS_BASE}{err}`" if err.startswith("/") else f"- {err}"
        )
    lines.append("")
    return "\n".join(lines)


def _write_summary(summary_file: str | None, content: str) -> None:
    target = summary_file or os.environ.get("GITHUB_STEP_SUMMARY")
    if target:
        with open(target, "a", encoding="utf-8") as f:
            f.write(content + "\n")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Validate public cloud.google.com docs against the Cluster Toolkit repo."
    )
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument(
        "--check-pr",
        action="store_true",
        help="Check changed files in a PR and print a Cloud Docs reminder table.",
    )
    mode.add_argument(
        "--check-live",
        action="store_true",
        help="Fetch live cloud.google.com pages and fail (exit 1) if drift is detected.",
    )
    parser.add_argument(
        "--fail-on-impact",
        action="store_true",
        help=(
            "With --check-pr, exit 1 when Cloud-Doc-backed files are modified unless "
            "the PR description includes a 'Cloud-Docs:' acknowledgment line."
        ),
    )
    parser.add_argument(
        "--pr-body",
        default=None,
        help="PR description body to inspect for a 'Cloud-Docs:' acknowledgment (defaults to $PR_BODY).",
    )
    parser.add_argument(
        "--base-ref",
        default="origin/develop",
        help="Base git ref for --check-pr diff (default: origin/develop).",
    )
    parser.add_argument(
        "--changed-files",
        nargs="*",
        default=None,
        help="Explicit list of changed files for --check-pr (overrides git diff).",
    )
    parser.add_argument(
        "--local-dir",
        default=None,
        help="Read Cloud Docs HTML/Markdown pages from a local directory instead of HTTPS.",
    )
    parser.add_argument(
        "--summary-file",
        default=None,
        help="Optional path to append GitHub Markdown summary (defaults to $GITHUB_STEP_SUMMARY).",
    )
    args = parser.parse_args(argv)

    if args.check_pr:
        changed = (
            args.changed_files
            if args.changed_files is not None
            else get_changed_files_from_git(base_ref=args.base_ref)
        )
        impacts = match_pr_cloud_docs_impacts(changed)
        pr_body = (
            args.pr_body if args.pr_body is not None else os.environ.get("PR_BODY")
        )
        ack = extract_cloud_docs_ack(pr_body)
        should_fail = bool(impacts and args.fail_on_impact and not ack)
        md = format_pr_reminder_markdown(impacts, ack=ack)
        if impacts and not ack:
            _emit_github_annotations(impacts)
        print(md, file=sys.stderr if should_fail else sys.stdout)
        _write_summary(args.summary_file, md)
        return 1 if should_fail else 0

    # Default to --check-live when neither flag is specified.
    def fetcher(slug: str) -> str:
        return fetch_page(slug, local_dir=args.local_dir)

    errors = validate_live_cloud_docs(fetcher=fetcher, repo_root=REPO_ROOT)
    md = format_live_drift_markdown(errors)
    print(md, file=sys.stderr if errors else sys.stdout)
    _write_summary(args.summary_file, md)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
