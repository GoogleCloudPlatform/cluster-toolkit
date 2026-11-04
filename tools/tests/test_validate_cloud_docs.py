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
"""Unit tests for tools/validate_cloud_docs.py."""

import contextlib
import io
import os
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
import validate_cloud_docs as vcd  # noqa: E402


def _compliant_pages() -> dict[str, str]:
    """Returns a complete set of compliant synthetic HTML pages for all 15 monitored slugs."""
    cmd_root = " ".join(sorted(vcd.extract_cli_subcommands("cmd")))
    cmd_job = " ".join(sorted(vcd.extract_cli_subcommands("cmd/job")))
    cmd_cluster = " ".join(sorted(vcd.extract_cli_subcommands("cmd/cluster")))
    return {
        "/cluster-toolkit/docs/overview": (
            "<article>Cluster Toolkit supports Managed Lustre, TPU 7x, "
            "gcluster explain, and gcluster job.</article>"
        ),
        "/cluster-toolkit/docs/setup/configure-environment": (
            "<article>gcloud services enable compute.googleapis.com "
            "lustre.googleapis.com servicenetworking.googleapis.com</article>"
        ),
        "/cluster-toolkit/docs/setup/cluster-blueprint-catalog": (
            "<article>Catalog of blueprints with llms-full.txt, tpu7x, slinky, "
            "gke-consumption-options, and "
            "https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples/hpc-slurm.yaml"
            "</article>"
        ),
        "/cluster-toolkit/docs/quickstarts/slurm-cluster": (
            "<article>Quickstart using examples/hpc-slurm.yaml</article>"
        ),
        "/cluster-toolkit/docs/deploy/slurm/create-a3-ultra-cluster": (
            "<article>Requires Managed Lustre (lustre.googleapis.com) and Cluster Toolkit v1.103.0+."
            "<pre>make &amp;&amp; ./gcluster --version</pre></article>"
        ),
        "/cluster-toolkit/docs/deploy/slurm/create-a4-cluster": (
            "<article>Requires Managed Lustre (lustre.googleapis.com) and Cluster Toolkit v1.103.0+."
            "<pre>make &amp;&amp; ./gcluster --version</pre></article>"
        ),
        "/cluster-toolkit/docs/deploy/slurm/create-a4x-cluster": (
            "<article>Requires Managed Lustre (lustre.googleapis.com) and Cluster Toolkit v1.103.0+."
            "<pre>make &amp;&amp; ./gcluster --version</pre></article>"
        ),
        "/cluster-toolkit/docs/deploy/slurm/create-a4x-max-cluster": (
            "<article>Requires Managed Lustre (lustre.googleapis.com) and Cluster Toolkit v1.103.0+."
            "<pre>make &amp;&amp; ./gcluster --version</pre></article>"
        ),
        "/cluster-toolkit/docs/deploy/gke/create-a3-ultra-cluster": (
            "<article>Configure reservation_affinity with SPECIFIC_RESERVATION."
            "<pre>terraform_backend_defaults:\n  type: gcs</pre></article>"
        ),
        "/cluster-toolkit/docs/deploy/gke/create-a4-cluster": (
            "<article>Configure reservation_affinity with SPECIFIC_RESERVATION."
            "<pre>terraform_backend_defaults:\n  type: gcs</pre></article>"
        ),
        "/cluster-toolkit/docs/deploy/gke/create-a4x-cluster": (
            "<article>Configure reservation in gke-a4x-deployment.yaml."
            "<pre>terraform_backend_defaults:\n  type: gcs</pre></article>"
        ),
        "/cluster-toolkit/docs/reference/gcluster": f"<article>{cmd_root}</article>",
        "/cluster-toolkit/docs/reference/gcluster-job": f"<article>{cmd_job}</article>",
        "/cluster-toolkit/docs/reference/gcluster-cluster": f"<article>{cmd_cluster}</article>",
        "/ai-hypercomputer/docs/create/gke-ai-hypercompute": (
            "<article>We recommend Cluster Toolkit (gcluster) for RDMA clusters. "
            "gcloud container clusters create ...</article>"
        ),
    }


class TestValidateCloudDocs(unittest.TestCase):

    def test_extract_article_ignores_nav_outside_article(self):
        raw = (
            "<nav>hpc-centos-7 HIGH_SCALE_SSD</nav>"
            "<article><p>Clean article content</p></article>"
            "<footer>v1.62.0</footer>"
        )
        self.assertEqual(vcd.html_to_text(raw), "Clean article content")

    def test_check_slurm_gpu_page_detects_filestore_and_old_version(self):
        slug = "/cluster-toolkit/docs/deploy/slurm/create-a3-ultra-cluster"
        bp = "examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml"
        stale_html = (
            "<article>Request 10 TiB of HIGH_SCALE_SSD Filestore instance quota. "
            "Requires Cluster Toolkit v1.62.0. <pre>./gcluster --version</pre></article>"
        )
        errs = vcd.check_slurm_gpu_page(slug, bp, stale_html)
        self.assertEqual(len(errs), 4)
        self.assertTrue(any("Filestore" in e for e in errs))
        self.assertTrue(any("missing Managed Lustre" in e for e in errs))
        self.assertTrue(any("v1.62.0" in e for e in errs))
        self.assertTrue(any("without running 'make' first" in e for e in errs))

        clean_html = (
            "<article>Request 36 TiB Managed Lustre quota (lustre.googleapis.com). "
            "Requires v1.103.0+. <pre>make &amp;&amp; ./gcluster --version</pre></article>"
        )
        self.assertEqual(vcd.check_slurm_gpu_page(slug, bp, clean_html), [])

    def test_check_gke_gpu_page_detects_reservation_destroy_and_pre_paths(self):
        slug = "/cluster-toolkit/docs/deploy/gke/create-a3-ultra-cluster"
        bp = "examples/gke-a3-ultragpu/gke-a3-ultragpu.yaml"
        dep = "examples/gke-a3-ultragpu/gke-a3-ultragpu-deployment.yaml"
        stale_html = (
            "<article>For the reservation field, provide the name of your reservation."
            "<pre>kubectl apply -f ./examples/deleted-sample-workloads/missing-job.yaml\n"
            "./gcluster destroy CLUSTER_NAME</pre>"
            "</article>"
        )
        errs = vcd.check_gke_gpu_page(slug, bp, dep, stale_html)
        self.assertEqual(len(errs), 3)
        self.assertTrue(any("reservation_affinity" in e for e in errs))
        self.assertTrue(any("gcluster destroy CLUSTER_NAME" in e for e in errs))
        self.assertTrue(any("examples/deleted-sample-workloads/missing-job.yaml" in e for e in errs))

        clean_html = (
            "<article>Set reservation_affinity under vars."
            "<pre>./gcluster deploy -d examples/gke-a3-ultragpu/gke-a3-ultragpu-deployment.yaml "
            "examples/gke-a3-ultragpu/gke-a3-ultragpu.yaml</pre></article>"
        )
        self.assertEqual(vcd.check_gke_gpu_page(slug, bp, dep, clean_html), [])

        # gke-a4x uses flat $(vars.reservation) inside its module reservation_affinity block
        a4x_slug = "/cluster-toolkit/docs/deploy/gke/create-a4x-cluster"
        a4x_bp = "examples/gke-a4x/gke-a4x.yaml"
        a4x_dep = "examples/gke-a4x/gke-a4x-deployment.yaml"
        a4x_stale = "<article>Set reservation_affinity under vars.</article>"
        a4x_errs = vcd.check_gke_gpu_page(a4x_slug, a4x_bp, a4x_dep, a4x_stale)
        self.assertTrue(any("uses flat 'reservation'" in e for e in a4x_errs))

    def test_check_blueprint_catalog_page_detects_removed_centos7_and_daos(self):
        slug = "/cluster-toolkit/docs/setup/cluster-blueprint-catalog"
        stale_html = (
            "<article>hpc-centos-7 †This OS image has DAOS drivers pre-installed.</article>"
        )
        errs = vcd.check_blueprint_catalog_page(slug, stale_html)
        self.assertEqual(len(errs), 3)
        self.assertTrue(any("hpc-centos-7" in e for e in errs))
        self.assertTrue(any("DAOS drivers" in e for e in errs))
        self.assertTrue(any("llms.txt" in e for e in errs))

    def test_check_configure_environment_page_detects_missing_apis(self):
        slug = "/cluster-toolkit/docs/setup/configure-environment"
        stale_html = "<article>gcloud services enable compute.googleapis.com file.googleapis.com</article>"
        errs = vcd.check_configure_environment_page(slug, stale_html)
        self.assertEqual(len(errs), 2)
        self.assertTrue(any("lustre.googleapis.com" in e for e in errs))
        self.assertTrue(any("servicenetworking.googleapis.com" in e for e in errs))

    def test_check_cli_reference_page_detects_missing_explain(self):
        slug = "/cluster-toolkit/docs/reference/gcluster"
        stale_html = (
            "<article>create deploy destroy expand export-outputs "
            "import-inputs telemetry completion</article>"
        )
        errs = vcd.check_cli_reference_page(slug, "cmd", stale_html)
        self.assertTrue(any("'explain'" in e for e in errs))
        self.assertTrue(any("'job'" in e for e in errs))
        self.assertTrue(any("'cluster'" in e for e in errs))

        # When <dt><code> tags are present, prose mentions do not mask missing subcommands
        # and removed/stale <dt><code> entries are also flagged
        dt_html = (
            "<article><p>You can deploy or explain blueprints.</p>"
            "<dl><dt><a href='#'><code>create</code></a></dt>"
            "<dt><a href='#'><code>obsolete-removed-cmd</code></a></dt></dl></article>"
        )
        dt_errs = vcd.check_cli_reference_page(slug, "cmd", dt_html)
        self.assertTrue(any("'explain'" in e for e in dt_errs))
        self.assertTrue(any("'deploy'" in e for e in dt_errs))
        self.assertTrue(any("documented CLI subcommand 'obsolete-removed-cmd'" in e for e in dt_errs))

    def test_check_github_blob_links_detects_deleted_files(self):
        slug = "/cluster-toolkit/docs/setup/cluster-blueprint-catalog"
        html = (
            "<article>"
            "https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples/hpc-slurm.yaml "
            "https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples/non-existent-deleted.yaml"
            "</article>"
        )
        errs = vcd.check_github_blob_links(slug, html)
        self.assertEqual(len(errs), 1)
        self.assertIn("examples/non-existent-deleted.yaml", errs[0])

    def test_check_pr_mode_maps_files_and_supports_fail_on_impact_and_pr_body_ack(self):
        with tempfile.NamedTemporaryFile("w+", encoding="utf-8", delete=False) as tf:
            summary_path = tf.name
        try:
            buf, err_buf = io.StringIO(), io.StringIO()
            old_gha = os.environ.get("GITHUB_ACTIONS")
            os.environ["GITHUB_ACTIONS"] = "true"
            try:
                with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(err_buf):
                    rc = vcd.main(
                        [
                            "--check-pr",
                            "--changed-files",
                            "examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml",
                            "cmd/explain.go",
                            "cmd/explain_test.go",
                            "cmd/README.md",
                            "cmd/job/list.go",
                            "--pr-body",
                            "",
                            "--summary-file",
                            summary_path,
                        ]
                    )
                    rc_fail = vcd.main(
                        [
                            "--check-pr",
                            "--fail-on-impact",
                            "--changed-files",
                            "cmd/explain.go",
                            "--pr-body",
                            "Normal PR description without ack",
                            "--summary-file",
                            summary_path,
                        ]
                    )
                    rc_acked = vcd.main(
                        [
                            "--check-pr",
                            "--fail-on-impact",
                            "--changed-files",
                            "cmd/explain.go",
                            "--pr-body",
                            "Summary of changes\nCloud-Docs: b/573807418 (planned update)",
                            "--summary-file",
                            summary_path,
                        ]
                    )
                    rc_clean = vcd.main(
                        [
                            "--check-pr",
                            "--fail-on-impact",
                            "--changed-files",
                            "cmd/explain_test.go",
                            "cmd/README.md",
                            "--pr-body",
                            "",
                            "--summary-file",
                            summary_path,
                        ]
                    )
            finally:
                if old_gha is None:
                    os.environ.pop("GITHUB_ACTIONS", None)
                else:
                    os.environ["GITHUB_ACTIONS"] = old_gha
            self.assertEqual(rc, 0)
            self.assertEqual(rc_fail, 1)
            self.assertEqual(rc_acked, 0)
            self.assertEqual(rc_clean, 0)
            self.assertIn("::warning file=cmd/explain.go", buf.getvalue())
            with open(summary_path, encoding="utf-8") as f:
                summary = f.read()
            self.assertIn("create-a3-ultra-cluster", summary)
            self.assertIn("/cluster-toolkit/docs/reference/gcluster", summary)
            self.assertIn("/cluster-toolkit/docs/reference/gcluster-job", summary)
            self.assertIn("b/573807418 (planned update)", summary)
            self.assertNotIn("explain_test.go", summary)
        finally:
            os.remove(summary_path)

    def test_extract_cloud_docs_ack_edge_cases(self):
        cases = {
            "Cloud-Docs: none": "none",
            "* Cloud-Docs: b/123": "b/123",
            "- Cloud-Docs: none": "none",
            "**Cloud-Docs:** none": "none",
            "Desc\r\nCloud-Docs: none\r\n": "none",
            "Cloud-Docs:\n### Testing": None,
            "Cloud-Docs: <bug or none>": None,
            "Cloud-Docs: <!-- bug or none -->": None,
            "<!--\nCloud-Docs: b/1\n-->": None,
        }
        for body, want in cases.items():
            with self.subTest(body=body):
                self.assertEqual(vcd.extract_cloud_docs_ack(body), want)

    def test_get_changed_files_reports_both_sides_of_a_cross_dir_move(self):
        with tempfile.TemporaryDirectory() as repo:
            def git(*args: str) -> None:
                subprocess.run(["git", *args], cwd=repo, check=True, capture_output=True)

            git("init", "-q", "-b", "develop")
            git("config", "user.email", "t@example.com")
            git("config", "user.name", "t")
            os.makedirs(os.path.join(repo, "examples/gke-a4"))
            with open(os.path.join(repo, "examples/gke-a4/gke-a4.yaml"), "w", encoding="utf-8") as f:
                f.write("blueprint_name: gke-a4\n" * 20)
            git("add", "-A")
            git("commit", "-qm", "base")
            git("checkout", "-qb", "feature")
            os.makedirs(os.path.join(repo, "examples/archive"))
            git("mv", "examples/gke-a4/gke-a4.yaml", "examples/archive/gke-a4.yaml")
            git("commit", "-qm", "move")
            changed = vcd.get_changed_files_from_git(base_ref="develop", repo_root=repo)
            self.assertIn("examples/gke-a4/gke-a4.yaml", changed)
            self.assertTrue(vcd.match_pr_cloud_docs_impacts(changed))

    def test_resilience_and_community_code_block_paths(self):
        # 1. community/examples/ path in <pre> must not be truncated to examples/
        article = (
            "<article><pre>./gcluster deploy "
            "community/examples/apptainer-artifact-registry-openfoam.yaml</pre></article>"
        )
        self.assertEqual(
            vcd.check_code_block_paths("/cluster-toolkit/docs/overview", article),
            [],
        )

        # 2. Failed fetch is cached so each failing slug is only fetched once
        calls: list[str] = []

        def failing_fetcher(slug: str) -> str:
            calls.append(slug)
            raise RuntimeError("network timeout")

        errs = vcd.validate_live_cloud_docs(fetcher=failing_fetcher)
        self.assertEqual(len(calls), len(vcd.ALL_MONITORED_PAGES))
        self.assertEqual(len(errs), len(vcd.ALL_MONITORED_PAGES))

        # 3. Missing repo files in repo_root report clean errors instead of crashing
        pages = _compliant_pages()
        with tempfile.TemporaryDirectory() as empty_root:
            missing_errs = vcd.validate_live_cloud_docs(
                fetcher=lambda s: pages[s], repo_root=empty_root
            )
            self.assertTrue(any("stale repo mapping" in e for e in missing_errs))

    def test_validate_live_cloud_docs_passes_on_compliant_pages_and_fails_on_drift(self):
        pages = _compliant_pages()
        errs = vcd.validate_live_cloud_docs(fetcher=lambda s: pages[s])
        self.assertEqual(errs, [])

        # Introduce drift in one page and verify --local-dir exits 1 without polluting GITHUB_STEP_SUMMARY
        with tempfile.TemporaryDirectory() as tmpdir:
            summary_file = os.path.join(tmpdir, "summary.md")
            for slug, content in pages.items():
                full = os.path.join(tmpdir, slug.lstrip("/") + ".html")
                os.makedirs(os.path.dirname(full), exist_ok=True)
                with open(full, "w", encoding="utf-8") as f:
                    f.write(content)
            out_buf, err_buf = io.StringIO(), io.StringIO()
            with contextlib.redirect_stdout(out_buf), contextlib.redirect_stderr(err_buf):
                self.assertEqual(
                    vcd.main(["--check-live", "--local-dir", tmpdir, "--summary-file", summary_file]),
                    0,
                )

            # Corrupt configure-environment
            bad_env = os.path.join(
                tmpdir, "cluster-toolkit/docs/setup/configure-environment.html"
            )
            with open(bad_env, "w", encoding="utf-8") as f:
                f.write("<article>gcloud services enable compute.googleapis.com</article>")
            with contextlib.redirect_stdout(out_buf), contextlib.redirect_stderr(err_buf):
                self.assertEqual(
                    vcd.main(["--check-live", "--local-dir", tmpdir, "--summary-file", summary_file]),
                    1,
                )

    def test_fail_closed_git_diff_and_pr_mapping_guard(self):
        # A git diff that cannot run yields None, and --check-pr exits 2 (never "no impacts").
        with tempfile.TemporaryDirectory() as repo:
            subprocess.run(["git", "init", "-q", "-b", "lone"], cwd=repo, check=True)
            for msg in ("one", "two"):
                subprocess.run(
                    [
                        "git",
                        "-c",
                        "user.email=t@example.com",
                        "-c",
                        "user.name=t",
                        "-c",
                        "commit.gpgsign=false",
                        "commit",
                        "-q",
                        "--allow-empty",
                        "-m",
                        msg,
                    ],
                    cwd=repo,
                    check=True,
                )
            with contextlib.redirect_stderr(io.StringIO()):
                self.assertIsNone(
                    vcd.get_changed_files_from_git(
                        base_ref="origin/develop", repo_root=repo
                    )
                )
        with mock.patch.object(vcd, "get_changed_files_from_git", return_value=None), contextlib.redirect_stderr(
            io.StringIO()
        ):
            self.assertEqual(
                vcd.main(["--check-pr", "--fail-on-impact", "--pr-body", ""]), 2
            )

        # Every PR_PATH_TO_CLOUD_DOCS prefix missing from repo_root is reported.
        with tempfile.TemporaryDirectory() as empty_root:
            errs = vcd.validate_live_cloud_docs(
                fetcher=lambda s: "<article></article>", repo_root=empty_root
            )
        stale = [
            e
            for e in errs
            if e.startswith("PR_PATH_TO_CLOUD_DOCS: stale repo mapping")
        ]
        self.assertEqual(len(stale), len(vcd.PR_PATH_TO_CLOUD_DOCS))


if __name__ == "__main__":
    unittest.main()
