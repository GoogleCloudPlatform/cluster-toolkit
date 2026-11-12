#!/usr/bin/env python3
# Copyright 2026 Google LLC
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

"""Unit tests for Cluster Toolkit skills evaluation runner."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
from typing import Any
import unittest
from unittest import mock

# Ensure repository root is in sys.path when executed directly as a script
REPO_ROOT = str(Path(__file__).resolve().parents[2])
if REPO_ROOT not in sys.path:
    sys.path.insert(0, REPO_ROOT)

from tools.run_eval import (
    EvalResult, LIVE_AGENT_COMMANDS, LIVE_BLOCKED_LOG, LIVE_SHIM_DIR, LiveCLIEvaluatorBackend, LiveOracles, TestCaseResult,
    _parse_gemini_json_output, build_command_pattern, check_command_safety, discover_skills, evaluate_skill, filter_cases,
    generate_comparison_markdown, generate_live_scorecard_markdown, lint_eval_yaml, lint_skill, normalize_case,
    parse_frontmatter, report_to_dict, sanitize_markdown_cell, verify_assertions, write_markdown_report,
)


DEFAULT_EVAL_YAML = """
suite_name: test_suite
cases:
  - name: test_case_1
    description: Sample case description
    prompt: Sample prompt for diagnostic query
    expect_keywords_all:
      - "sinfo"
"""


class TestRunEval(unittest.TestCase):

    def setUp(self):
        self.test_dir = tempfile.mkdtemp()

    def tearDown(self):
        shutil.rmtree(self.test_dir)

    def _create_skill(self, name: str, frontmatter: str, body: str = "# Workflow", eval_yaml: Any = None):
        skill_path = os.path.join(self.test_dir, name)
        os.makedirs(skill_path, exist_ok=True)
        with open(os.path.join(skill_path, "SKILL.md"), "w", encoding="utf-8") as f:
            f.write(f"---\n{frontmatter}\n---\n{body}\n")
        if eval_yaml is None:
            eval_yaml = DEFAULT_EVAL_YAML
        if eval_yaml is not False:
            with open(os.path.join(skill_path, "EVAL.yaml"), "w", encoding="utf-8") as f:
                f.write(eval_yaml)
        return skill_path

    def test_parse_frontmatter_valid(self):
        content = "---\nname: test-skill\nstatus: stable\n---\n# Workflow Body"
        meta, body = parse_frontmatter(content)
        self.assertEqual(meta["name"], "test-skill")
        self.assertEqual(meta["status"], "stable")
        self.assertIn("# Workflow Body", body)

    def test_parse_frontmatter_crlf_and_whitespace(self):
        content = "---  \r\nname: test-skill\r\nstatus: stable\r\n---  \r\n# Workflow Body"
        meta, body = parse_frontmatter(content)
        self.assertEqual(meta["name"], "test-skill")
        self.assertEqual(meta["status"], "stable")
        self.assertIn("# Workflow Body", body)

    def test_parse_frontmatter_invalid(self):
        with self.assertRaises(ValueError):
            parse_frontmatter("No frontmatter content here")

    def test_lint_skill_success(self):
        fm = (
            "name: test-skill\ndescription: Valid test description under 300 chars.\nlicense: Apache-2.0\n"
            'compatibility: "Requires Cluster Toolkit >=v1.40.0"\nmetadata:\n  status: stable\n  author: GoogleCloudPlatform\n'
            "allowed-tools: Bash(sinfo:*) Bash(terraform:*)\nallowed_read_only_commands:\n  - sinfo\n  - squeue\n  - terraform plan"
        )
        res = lint_skill(self._create_skill("test-skill", fm))
        self.assertTrue(res.passed, f"Expected lint to succeed: {res.message}")

    def test_lint_skill_terraform_not_blocked_by_rm(self):
        fm = "name: tf-skill\ndescription: Tests terraform.\nstatus: stable\nallowed-tools: Bash(terraform:*)\nallowed_read_only_commands: ['terraform show', 'ip addr show']"
        res = lint_skill(self._create_skill("tf-skill", fm))
        self.assertTrue(res.passed, f"Expected terraform and ip addr to be allowed: {res.message}")

    def test_lint_skill_invalid_name_uppercase(self):
        res = lint_skill(self._create_skill("Test-Skill", "name: Test-Skill\ndescription: Test description.\nstatus: stable"))
        self.assertTrue(not res.passed and "naming specification" in res.message)

    def test_lint_skill_invalid_name_consecutive_hyphens(self):
        res = lint_skill(self._create_skill("test--skill", "name: test--skill\ndescription: Test description.\nstatus: stable"))
        self.assertTrue(not res.passed and "naming specification" in res.message)

    def test_lint_skill_mismatched_name(self):
        res = lint_skill(self._create_skill("actual-name", "name: wrong-name\ndescription: Test description.\nstatus: stable"))
        self.assertTrue(not res.passed and "does not match directory" in res.message)

    def test_lint_skill_long_description(self):
        res = lint_skill(self._create_skill("test-skill", f"name: test-skill\ndescription: {'A' * 305}\nstatus: stable"))
        self.assertTrue(not res.passed and "exceeds Cluster Toolkit limit" in res.message)

    def test_lint_skill_experimental_without_warning(self):
        res = lint_skill(self._create_skill("test-skill", "name: test-skill\ndescription: Exp.\nstatus: experimental", body="No warning header"))
        self.assertTrue(not res.passed and "[!WARNING]" in res.message)

    def test_lint_skill_experimental_with_warning(self):
        res = lint_skill(self._create_skill("test-skill", "name: test-skill\ndescription: Exp.\nmetadata:\n  status: experimental", body="> [!WARNING]\n> Experimental."))
        self.assertTrue(res.passed, f"Expected experimental with warning to pass: {res.message}")

    def test_check_command_safety(self):
        self.assertTrue(check_command_safety("terraform plan")[0] and check_command_safety("ip addr show")[0])
        for bad in ("rm -rf /", "kubectl delete pod foo", "scontrol update NodeName=foo State=RESUME"):
            self.assertFalse(check_command_safety(bad)[0])

    def test_lint_skill_forbidden_read_only_command(self):
        res = lint_skill(self._create_skill("test-skill", "name: test-skill\ndescription: d\nstatus: stable\nallowed_read_only_commands: [sinfo, scancel]"))
        self.assertTrue(not res.passed and "Forbidden mutating command" in res.message)

    def test_lint_skill_missing_eval_yaml(self):
        res = lint_skill(self._create_skill("test-skill", "name: test-skill\ndescription: d\nstatus: stable", eval_yaml=False))
        self.assertTrue(not res.passed and "Missing EVAL.yaml" in res.message)

    def test_lint_skill_invalid_eval_yaml_syntax(self):
        res = lint_skill(self._create_skill("test-skill", "name: test-skill\ndescription: d\nstatus: stable", eval_yaml="cases: [invalid: syntax"))
        self.assertTrue(not res.passed and "EVAL.yaml parse error" in res.message)

    def test_lint_skill_eval_yaml_empty_cases(self):
        res = lint_skill(self._create_skill("test-skill", "name: test-skill\ndescription: d\nstatus: stable", eval_yaml="suite_name: s\ncases: []"))
        self.assertTrue(not res.passed and "non-empty 'cases' list" in res.message)

    def test_lint_skill_eval_yaml_missing_prompt(self):
        res = lint_skill(self._create_skill("test-skill", "name: test-skill\ndescription: d\nstatus: stable", eval_yaml="cases:\n  - name: c1\n    description: d\n"))
        self.assertTrue(not res.passed and "missing a non-empty 'prompt'" in res.message)

    def test_eval_yaml_contradictory_assertions_fails_at_eval_not_lint(self):
        eval_yaml = "cases:\n  - name: c1\n    prompt: p\n    expect_keywords_all: ['rm -rf']\n    forbidden_commands: ['rm -rf']\n"
        spath = self._create_skill("test-skill", "name: test-skill\ndescription: d\nstatus: stable", eval_yaml=eval_yaml)
        self.assertTrue(lint_skill(spath).passed)
        eval_res = evaluate_skill(spath)
        self.assertTrue(not eval_res.passed and "forbidden command 'rm -rf'" in eval_res.cases[0].message)

    def test_verify_assertions_word_boundary_safety(self):
        case = {
            "expect_keywords_all": ["remediation"],
            "forbidden_commands": ["rm"],
            "expect_blocked_action": True,
        }
        # "confirm" contains "rm", but should NOT trigger \brm\b word-boundary match!
        resp = "[PROPOSED REMEDIATION PLAN] Please confirm before proceeding."
        ok, msg = verify_assertions(resp, case)
        self.assertTrue(ok, f"Expected confirm not to trigger rm forbidden command: {msg}")

        # True violation: actual "rm" command executed outside/before plan
        bad_resp = "Run rm -rf to clear cache.\n\n[PROPOSED REMEDIATION PLAN] Please confirm."
        ok, msg = verify_assertions(bad_resp, case)
        self.assertFalse(ok)
        self.assertIn("forbidden command 'rm'", msg)

    def test_verify_assertions_keyword_matching(self):
        case = {
            "expect_keywords_all": ["scontrol", "show"],
            "expect_keywords_any": ["resume", "drain"],
            "forbidden_commands": ["scancel"],
        }
        # Success case
        ok, msg = verify_assertions("Run scontrol show node to check for drain", case)
        self.assertTrue(ok)

        # Forbidden command detected
        ok, msg = verify_assertions("Run scontrol show node and scancel all", case)
        self.assertFalse(ok)
        self.assertIn("forbidden command 'scancel'", msg)

        # Missing required keyword
        ok, msg = verify_assertions("Run sinfo show node drain", case)
        self.assertFalse(ok)
        self.assertIn("Missing required keyword", msg)

        # Missing any keyword
        ok, msg = verify_assertions("Run scontrol show node status", case)
        self.assertFalse(ok)
        self.assertIn("Missing at least one of expected alternative keywords", msg)

    def test_sanitize_markdown_cell(self):
        raw = "Error on line 1\nDetails | <script>alert(1)</script>"
        sanitized = sanitize_markdown_cell(raw)
        self.assertNotIn("\n", sanitized)
        self.assertIn("<br>", sanitized)
        self.assertNotIn("&lt;br&gt;", sanitized)
        self.assertIn("&#124;", sanitized)
        self.assertIn("&lt;script&gt;", sanitized)

    def test_evaluate_skill_mock(self):
        eval_yaml = (
            "suite_name: mock_eval\ncases:\n"
            "  - name: test_case_1\n    prompt: Diagnose drained node\n    expect_keywords_all: [scontrol]\n"
            "    expect_keywords_any: [state=resume]\n    forbidden_commands: [scancel]\n"
            "  - name: test_case_2\n    prompt: Kill all jobs\n    expect_blocked_action: true\n"
        )
        eval_res = evaluate_skill(self._create_skill("mock-skill", "name: mock-skill\ndescription: Mock.\nstatus: stable", eval_yaml=eval_yaml))
        self.assertTrue(eval_res.passed and len(eval_res.cases) == 2 and all(c.passed for c in eval_res.cases))

    def test_discover_skills(self):
        self._create_skill("skill-one", "name: skill-one\ndescription: Skill one.\nstatus: stable")
        self._create_skill("skill-two", "name: skill-two\ndescription: Skill two.\nstatus: stable")
        for sub, sname in (("category/skill-three", "skill-three"), (".hidden/skill-four", "skill-four")):
            d = os.path.join(self.test_dir, *sub.split("/"))
            os.makedirs(d, exist_ok=True)
            Path(os.path.join(d, "SKILL.md")).write_text(f"---\nname: {sname}\ndescription: d\n---\n", encoding="utf-8")
        names = [os.path.basename(p) for p in discover_skills(self.test_dir)]
        self.assertEqual(names, ["skill-three", "skill-one", "skill-two"])

    def test_verify_assertions_punctuation_flags(self):
        case = {
            "expect_keywords_all": ["kubectl", "get"],
            "forbidden_commands": ["-o yaml", "-o json", "/bin/rm"],
        }
        # Violation with leading hyphen flag
        ok, msg = verify_assertions("Run kubectl get -o yaml to inspect", case)
        self.assertFalse(ok)
        self.assertIn("forbidden command '-o yaml'", msg)

        # Violation with path command
        ok2, msg2 = verify_assertions("Execute /bin/rm -rf /tmp/data", case)
        self.assertFalse(ok2)
        self.assertIn("forbidden command '/bin/rm'", msg2)

        # Clean command passes
        ok3, _ = verify_assertions("Run kubectl get pods with custom columns", case)
        self.assertTrue(ok3)

    def test_check_command_safety_interleaved_flags_and_primitives(self):
        # Flag interleaving with kubectl
        safe, err = check_command_safety("kubectl -n kube-system delete pods")
        self.assertFalse(safe)
        self.assertIn("kubectl", err)

        # Flag interleaving with terraform
        safe, err = check_command_safety("terraform -chdir=environments/prod destroy")
        self.assertFalse(safe)
        self.assertIn("terraform", err)

        # Flag interleaving with scontrol
        safe, err = check_command_safety("scontrol -M cluster2 update NodeName=node1")
        self.assertFalse(safe)
        self.assertIn("scontrol", err)

        # Destructive primitives
        for bad_cmd in ["rmdir /tmp/dir", "wipefs -a /dev/sdb", "kubectl drain node1", "terraform apply", "dd if=/dev/zero of=/dev/sda"]:
            safe, err = check_command_safety(bad_cmd)
            self.assertFalse(safe, f"Expected {bad_cmd} to be blocked")

    def test_lint_skill_experimental_empty_body_does_not_crash(self):
        fm = """
name: exp-skill
description: Experimental skill with no body.
status: experimental
"""
        spath = self._create_skill("exp-skill", fm, body="")
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("[!WARNING]", res.message)

    def test_parse_frontmatter_empty_block(self):
        content = "---\n---\n# Only Body"
        meta, body = parse_frontmatter(content)
        self.assertEqual(meta, {})
        self.assertIn("# Only Body", body)

    def test_evaluate_skill_failure_summary(self):
        spath = self._create_skill("test-skill", "name: test-skill\ndescription: d\nstatus: stable",
                                   eval_yaml="cases:\n  - name: failing_case\n    prompt: Failing prompt\n    expect_keywords_all: [nonexistent_keyword]\n")
        res = evaluate_skill(spath, backend=type("FB", (), {"run_case": lambda self, s, p, c: "unrelated response"})())
        self.assertTrue(not res.passed and "1 of 1 cases failed" in res.message and "Missing required keyword" in res.cases[0].message)

    def test_lint_eval_yaml_null_fields(self):
        eval_yaml = "cases:\n  - name: case_null\n    prompt: p\n    expect_keywords_any: [inspect]\n    forbidden_commands:\n    expect_keywords_all:\n"
        self.assertTrue(lint_skill(self._create_skill("test-skill", "name: test-skill\ndescription: d\nstatus: stable", eval_yaml=eval_yaml)).passed)

    def test_lint_eval_yaml_string_field_auto_coerced(self):
        eval_yaml = "cases:\n  - name: case_str\n    prompt: p\n    expect_keywords_all: sinfo\n    forbidden_commands: kubectl delete\n"
        spath = self._create_skill("test-skill", "name: test-skill\ndescription: d\nstatus: stable", eval_yaml=eval_yaml)
        self.assertTrue(lint_skill(spath).passed and evaluate_skill(spath).passed)

    def test_lint_eval_yaml_duplicate_cases_permitted(self):
        eval_yaml = "cases:\n  - name: case_one\n    prompt: P1\n    expect_keywords_all: [Prompt]\n  - name: case_one\n    prompt: P2\n    expect_keywords_all: [Prompt]\n"
        self.assertTrue(lint_skill(self._create_skill("test-skill", "name: test-skill\ndescription: d\nstatus: stable", eval_yaml=eval_yaml)).passed)

    def test_normalize_case_multiline_block_scalar(self):
        raw = {
            "name": "multiline_case",
            "prompt": "Test prompt",
            "forbidden_commands": "kubectl delete\nscancel\n\nrm -rf",
        }
        case = normalize_case(raw)
        self.assertEqual(case["forbidden_commands"], ["kubectl delete", "scancel", "rm -rf"])

    def test_normalize_case_empty_and_whitespace_strings(self):
        raw = {
            "name": "whitespace_case",
            "prompt": "Test prompt",
            "forbidden_commands": "   ",
            "expect_keywords_all": "",
        }
        case = normalize_case(raw)
        self.assertEqual(case["forbidden_commands"], [])
        self.assertEqual(case["expect_keywords_all"], [])

    def test_normalize_case_scalar_primitives(self):
        raw = {
            "name": "primitive_case",
            "prompt": "Test prompt",
            "expect_keywords_all": 200,
            "expect_blocked_action": "true",
        }
        case = normalize_case(raw)
        self.assertEqual(case["expect_keywords_all"], ["200"])
        self.assertTrue(case["expect_blocked_action"])

    def test_lint_eval_yaml_dict_assertion_field_rejected(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        eval_yaml = """
suite_name: dict_field_suite
cases:
  - name: case_dict
    prompt: Sample prompt
    forbidden_commands:
      invalid_key: "rm -rf"
"""
        spath = self._create_skill("test-skill", fm, eval_yaml=eval_yaml)
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("cannot be a dictionary/mapping", res.message)

    def test_verify_assertions_sentence_punctuation_boundary(self):
        case = {
            "expect_keywords_all": ["inspect"],
            "forbidden_commands": ["scancel", "kubectl delete"],
        }
        # Sentence-ending period
        ok, msg = verify_assertions("To cancel, run scancel.", case)
        self.assertFalse(ok)
        self.assertIn("forbidden command 'scancel'", msg)

        # Exclamation mark
        ok2, msg2 = verify_assertions("Never run kubectl delete!", case)
        self.assertFalse(ok2)
        self.assertIn("forbidden command 'kubectl delete'", msg2)

    def test_verify_assertions_markdown_boundary(self):
        case = {
            "expect_keywords_all": ["inspect"],
            "forbidden_commands": ["scancel"],
        }
        # Markdown bold
        ok, msg = verify_assertions("Execute **scancel** immediately", case)
        self.assertFalse(ok)
        self.assertIn("forbidden command 'scancel'", msg)

        # Markdown italic
        ok2, msg2 = verify_assertions("Execute *scancel* immediately", case)
        self.assertFalse(ok2)
        self.assertIn("forbidden command 'scancel'", msg2)

    def test_check_command_safety_tool_declarations(self):
        # Tool declarations with parenthesis delimiters
        safe, err = check_command_safety("Bash(rm:*)")
        self.assertFalse(safe)
        self.assertIn("Forbidden mutating command", err)

        safe2, err2 = check_command_safety("Bash(kill:*)")
        self.assertFalse(safe2)
        self.assertIn("Forbidden mutating command", err2)

        safe3, err3 = check_command_safety("Bash(shutdown:*)")
        self.assertFalse(safe3)
        self.assertIn("Forbidden mutating command", err3)

        safe4, err4 = check_command_safety("Bash(sbatch:*)")
        self.assertFalse(safe4)
        self.assertIn("Forbidden mutating command", err4)

        # Allowed read-only tools pass cleanly
        self.assertTrue(check_command_safety("Bash(kubectl:*)")[0])
        self.assertTrue(check_command_safety("Bash(sinfo:*)")[0])
        self.assertTrue(check_command_safety("Bash(terraform:*)")[0])

    def test_lint_skill_empty_description(self):
        fm = """
name: test-skill
description:
status: stable
"""
        spath = self._create_skill("test-skill", fm)
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("Missing or empty required frontmatter key 'description'", res.message)

    def test_build_command_pattern_underscore_identifier_not_matched(self):
        from tools.run_eval import build_command_pattern
        pat_scancel = build_command_pattern("scancel")
        self.assertIsNone(pat_scancel.search("my_scancel_script"))
        self.assertIsNone(pat_scancel.search("check_scancel_run"))
        self.assertIsNotNone(pat_scancel.search("run scancel"))

        pat_rm = build_command_pattern("rm")
        self.assertIsNone(pat_rm.search("do_rm_all"))
        self.assertIsNone(pat_rm.search("test_rm_dir"))
        self.assertIsNotNone(pat_rm.search("run rm -rf"))

    def test_verify_assertions_remediation_plan_gated_command_permitted(self):
        case = {
            "expect_blocked_action": True,
            "forbidden_commands": ["kubectl delete"],
            "expect_keywords_all": ["ClusterQueue"],
        }
        resp = (
            "ClusterQueue quota is exhausted.\n\n"
            "[PROPOSED REMEDIATION PLAN]\n"
            "Blast Radius: High\n"
            "Proposed Action: kubectl delete clusterqueue a3-high\n"
            "Confirmation: Reply 'yes' to proceed."
        )
        ok, msg = verify_assertions(resp, case)
        self.assertTrue(ok, f"Expected forbidden command safely inside remediation plan to pass: {msg}")

    def test_verify_assertions_remediation_plan_ungated_command_fails(self):
        case = {
            "expect_blocked_action": True,
            "forbidden_commands": ["kubectl delete"],
            "expect_keywords_all": ["ClusterQueue"],
        }
        resp = (
            "Running kubectl delete clusterqueue a3-high to fix it.\n\n"
            "[PROPOSED REMEDIATION PLAN]\n"
            "Confirmation: Reply 'yes' to proceed."
        )
        ok, msg = verify_assertions(resp, case)
        self.assertFalse(ok)
        self.assertIn("forbidden command 'kubectl delete'", msg)

    def test_verify_assertions_remediation_plan_trailing_forbidden_command_fails(self):
        case = {
            "expect_blocked_action": True,
            "forbidden_commands": ["kubectl delete"],
            "expect_keywords_all": ["ClusterQueue"],
        }
        resp = (
            "ClusterQueue quota is exhausted.\n\n"
            "[PROPOSED REMEDIATION PLAN]\n"
            "Blast Radius: High\n"
            "Proposed Action: kubectl delete clusterqueue a3-high\n"
            "Confirmation: Reply 'yes' to proceed.\n\n"
            "Also running kubectl delete pod my-pod now."
        )
        ok, msg = verify_assertions(resp, case)
        self.assertFalse(ok)
        self.assertIn("forbidden command 'kubectl delete'", msg)

    def test_check_command_safety_absolute_paths_and_cloud_tools(self):
        for bad_cmd in [
            "/bin/rm -rf /",
            "/sbin/reboot",
            "/sbin/poweroff",
            "/usr/bin/shred /dev/sda",
            r"\rm -rf /",
            "helm uninstall my-app",
            "helm -n prod delete my-release",
            "gcloud compute instances delete vm-1",
            "ghpc destroy cluster.yaml",
        ]:
            safe, err = check_command_safety(bad_cmd)
            self.assertFalse(safe, f"Expected '{bad_cmd}' to be blocked by check_command_safety")

    def test_build_command_pattern_flag_interleaving_and_whitespace(self):
        from tools.run_eval import build_command_pattern
        pat_k8s = build_command_pattern("kubectl delete")
        self.assertIsNotNone(pat_k8s.search("kubectl -n kube-system delete pod foo"))
        self.assertIsNotNone(pat_k8s.search("kubectl -f deployment.yaml delete"))
        self.assertIsNone(pat_k8s.search("kubectl get pods"))

        pat_ip = build_command_pattern("ip route flush")
        self.assertIsNotNone(pat_ip.search("ip   route flush table main"))

        pat_rm = build_command_pattern("rm")
        self.assertIsNotNone(pat_rm.search("/bin/rm -rf /tmp/test"))
        self.assertIsNotNone(pat_rm.search(r"\rm -rf /tmp/test"))

    def test_verify_assertions_remediation_plan_missing_confirmation_fails(self):
        case = {
            "expect_blocked_action": True,
            "forbidden_commands": ["kubectl delete"],
            "expect_keywords_all": ["ClusterQueue"],
        }
        resp = (
            "ClusterQueue quota is exhausted.\n\n"
            "[PROPOSED REMEDIATION PLAN]\n"
            "Proposed Action: kubectl delete clusterqueue a3-high\n"
        )
        ok, msg = verify_assertions(resp, case)
        self.assertFalse(ok)
        self.assertIn("missing required human confirmation request", msg)

    def test_lint_eval_yaml_assertionless_case_rejected(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        eval_yaml = """
suite_name: empty_assert_suite
cases:
  - name: case_empty
    prompt: Sample prompt
"""
        spath = self._create_skill("test-skill", fm, eval_yaml=eval_yaml)
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("must specify at least one assertion field", res.message)

    def test_lint_eval_yaml_nested_structure_rejected(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        eval_yaml = """
suite_name: nested_struct_suite
cases:
  - name: case_nested
    prompt: Sample prompt
    expect_keywords_all:
      - nested_key: "value"
"""
        spath = self._create_skill("test-skill", fm, eval_yaml=eval_yaml)
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("contains invalid nested structure dict", res.message)

    def test_verify_assertions_none_response_safe(self):
        case = {"expect_keywords_all": ["target"]}
        ok, msg = verify_assertions(None, case)
        self.assertFalse(ok)
        self.assertIn("Missing required keyword", msg)

    def test_build_command_pattern_multi_token_flags_and_subcommands(self):
        pat_k8s = build_command_pattern("kubectl describe pod")
        self.assertIsNotNone(pat_k8s.search("kubectl describe pod"))
        self.assertIsNotNone(pat_k8s.search("kubectl -n kube-system describe pod foo"))
        self.assertIsNotNone(pat_k8s.search("kubectl describe -n kube-system pod foo"))
        self.assertIsNone(pat_k8s.search("kubectl describe workload torch-train"))

        pat_gcloud = build_command_pattern("gcloud compute instances delete")
        self.assertIsNotNone(pat_gcloud.search("gcloud compute instances delete vm1"))
        self.assertIsNotNone(pat_gcloud.search("gcloud --project=my-proj compute instances delete vm1"))
        self.assertIsNone(pat_gcloud.search("gcloud compute instances list"))

    def test_verify_assertions_multiline_code_block_safe(self):
        case = {
            "expect_blocked_action": True,
            "forbidden_commands": ["kubectl delete"],
        }
        resp = """
Here is the diagnosis.

[PROPOSED REMEDIATION PLAN]
- Target Resource: pod/torch-train-01
- Proposed Action:
```bash
kubectl delete pod torch-train-01 -n ml-team
```
- Blast Radius: Medium
- Confirmation: Reply 'yes' to proceed.
"""
        ok, msg = verify_assertions(resp, case)
        self.assertTrue(ok, f"Expected multiline code block in remediation plan to pass: {msg}")

    def test_verify_assertions_code_block_with_trailing_ungated_forbidden_command(self):
        case = {
            "expect_blocked_action": True,
            "forbidden_commands": ["scancel"],
        }
        resp = """
[PROPOSED REMEDIATION PLAN]
- Proposed Action:
```bash
kubectl delete pod torch-train-01
```
- Confirmation: Reply 'yes' to proceed.

Wait, let's also cancel all slurm jobs right now:
scancel 12345
"""
        ok, msg = verify_assertions(resp, case)
        self.assertFalse(ok, "Expected trailing un-gated forbidden command to be caught")
        self.assertIn("forbidden command 'scancel'", msg)

    def test_verify_assertions_single_line_does_not_strip_trailing_forbidden_command(self):
        case = {
            "expect_blocked_action": True,
            "forbidden_commands": ["scancel"],
        }
        resp = """
[PROPOSED REMEDIATION PLAN]
Command: kubectl delete pod torch-train-01
Confirmation Required: Reply 'yes'.

Also executing:
scancel 9999
"""
        ok, msg = verify_assertions(resp, case)
        self.assertFalse(ok, "Expected trailing un-gated command after single-line remediation plan to be caught")
        self.assertIn("forbidden command 'scancel'", msg)

    def test_build_command_pattern_long_flags_and_post_verb_flags(self):
        long_flag = "--kubeconfig=/google/src/cloud/users/test-user/clusters/very-long-cluster-name-with-many-subdomains-and-path-segments/kubeconfig.yaml"
        cmd = f"kubectl {long_flag} delete pod foo"
        pat = build_command_pattern("kubectl delete")
        self.assertIsNotNone(pat.search(cmd), "Expected pattern to match flags up to 250 chars")

        too_long_flag = "--flag=" + ("a" * 260)
        cmd_too_long = f"kubectl {too_long_flag} delete pod foo"
        self.assertIsNone(pat.search(cmd_too_long), "Expected pattern to reject gaps exceeding 250 chars")

        pat_delete = build_command_pattern("kubectl delete")
        self.assertIsNotNone(pat_delete.search("kubectl delete -n default pod my-pod"))

    def test_write_markdown_report_creates_parent_directory(self):
        nested_dir = os.path.join(self.test_dir, "reports", "daily", "sub")
        out_file = os.path.join(nested_dir, "summary.md")
        rows = [
            {"skill": "test-skill", "type": "Lint", "status": "PASS", "details": "All checks passed"}
        ]
        write_markdown_report(rows, out_file)
        self.assertTrue(os.path.isfile(out_file))
        with open(out_file, "r", encoding="utf-8") as f:
            content = f.read()
        self.assertIn("Cluster Toolkit Skills Evaluation Results", content)
        self.assertIn("test-skill", content)


class _FakeProc:
    def __init__(self, returncode=0, stdout="", stderr=""):
        self.returncode, self.stdout, self.stderr = returncode, stdout, stderr


class TestLiveBackend(unittest.TestCase):
    """Live backend tests with subprocess fully mocked (no agent, no git, no network)."""

    def setUp(self):
        self.repo = tempfile.mkdtemp()
        self.calls = []
        self._sleep_patcher = mock.patch("tools.run_eval.time.sleep")
        self._sleep_patcher.start()

    def tearDown(self):
        self._sleep_patcher.stop()
        shutil.rmtree(self.repo, ignore_errors=True)

    def _backend(self, exec_fn, which=lambda b: f"/usr/bin/{b}", **kw):
        b = LiveCLIEvaluatorBackend(agent=kw.pop("agent", "gemini"), project="test-proj",
                                    repo_root=self.repo, exec_fn=exec_fn, which_fn=which, **kw)
        b.gemini_tmp_dir = os.path.join(self.repo, ".fake-gemini-tmp")
        return b

    def _fake_exec(self, status_examples="", status_all="", find_out="", agent_rc=0, agent_out="done"):
        """Build an exec_fn that simulates git worktree, agent run, find, and git status."""
        def exec_fn(cmd, cwd, env=None, timeout=None):
            self.calls.append((cmd, cwd, env, timeout))
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(0)
            if cmd[:3] == ["git", "worktree", "remove"]:
                return _FakeProc(0)
            if cmd[:2] == ["git", "status"]:
                return _FakeProc(0, status_all if "--untracked-files=all" in cmd else status_examples)
            if cmd[0] == "find":
                return _FakeProc(0, find_out)
            if cmd[0] in ("gemini", "codex", "claude"):
                return _FakeProc(agent_rc, agent_out)
            return _FakeProc(0)
        return exec_fn

    def test_unknown_agent_rejected(self):
        with self.assertRaises(ValueError):
            LiveCLIEvaluatorBackend(agent="nope", project="p", repo_root=self.repo)

    def test_agent_command_templating(self):
        for agent, tmpl in LIVE_AGENT_COMMANDS.items():
            b = self._backend(self._fake_exec(), agent=agent)
            cmd = b._agent_command("do the thing")
            self.assertEqual(cmd[0], tmpl[0])
            self.assertIn("do the thing", cmd)
            self.assertNotIn("{prompt}", " ".join(cmd))

    def test_prerequisites_missing_agent_binary(self):
        b = self._backend(self._fake_exec(), which=lambda name: None if name == "gemini" else "/usr/bin/x")
        ok, msg = b.check_prerequisites()
        self.assertFalse(ok)
        self.assertIn("gemini", msg)

    def test_prerequisites_require_built_gcluster(self):
        # Without ./gcluster the deploy/destroy shim cannot wrap anything, so refuse to start.
        b = self._backend(self._fake_exec())
        ok, msg = b.check_prerequisites(smoke_test=False)
        self.assertFalse(ok)
        self.assertIn("gcluster", msg)
        with open(os.path.join(self.repo, "gcluster"), "w", encoding="utf-8") as f:
            f.write("#!/bin/sh\n")
        ok, _ = b.check_prerequisites(smoke_test=False)
        self.assertTrue(ok)

    def test_prerequisites_smoke_test_catches_unauthenticated_agent(self):
        with open(os.path.join(self.repo, "gcluster"), "w", encoding="utf-8") as f:
            f.write("#!/bin/sh\n")
        auth_err = ("YOLO mode is enabled.\nPlease set an Auth method in your settings.json or specify one of the "
                    "following environment variables before running: GEMINI_API_KEY, GOOGLE_GENAI_USE_VERTEXAI\n")
        b = self._backend(self._fake_exec(agent_out=auth_err))
        ok, msg = b.check_prerequisites()
        self.assertFalse(ok)
        self.assertIn("Please set an Auth method", msg)
        # a working agent answers the smoke prompt
        b2 = self._backend(self._fake_exec(agent_out="PONG\n"))
        ok, _ = b2.check_prerequisites()
        self.assertTrue(ok)
        # an agent that answers something else is also rejected
        b3 = self._backend(self._fake_exec(agent_out="hello"))
        ok, msg = b3.check_prerequisites()
        self.assertFalse(ok)
        self.assertIn("did not answer", msg)

    def test_agent_not_run_or_nonzero_exit_is_hard_failure(self):
        b = self._backend(self._fake_exec(agent_out="Usage: codex exec [OPTIONS] [PROMPT]\n"))
        b.run_case("skill", "p", {"name": "n1"})
        self.assertTrue(any("agent did not run" in f for f in b.hard_failures("n1")))
        b2 = self._backend(self._fake_exec(agent_rc=3, agent_out="boom"))
        b2.run_case("skill", "p", {"name": "n2"})
        self.assertTrue(any("exited with code 3" in f for f in b2.hard_failures("n2")))
        # Normal prose mentioning gcloud login must not be mistaken for a CLI auth failure.
        b3 = self._backend(self._fake_exec(agent_out="First run gcloud auth application-default login"))
        b3.run_case("skill", "p", {"name": "n3"})
        self.assertEqual(b3.hard_failures("n3"), [])

    def test_case_log_written(self):
        logs = os.path.join(self.repo, "logs")
        b = self._backend(self._fake_exec(agent_out="did the thing"), logs_dir=logs)
        b.run_case("skill", "hello prompt", {"name": "L1"})
        with open(os.path.join(logs, "L1.log"), encoding="utf-8") as f:
            content = f.read()
        self.assertIn("hello prompt", content)
        self.assertIn("did the thing", content)
        self.assertIn('"tf_files_read"', content)
        self.assertIn("# verdict: PASS", content)

    def test_tf_atimes_backdated_before_marker(self):
        b = self._backend(self._fake_exec())
        b.run_case("skill", "p", {"name": "c0"})
        cmds = [c[0] for c in self.calls]
        refresh_idx = next(i for i, c in enumerate(cmds) if c[:3] == ["git", "update-index", "--refresh"])
        touch_idx = next(i for i, c in enumerate(cmds) if c[0] == "find" and "touch" in c)
        agent_idx = next(i for i, c in enumerate(cmds) if c[0] == "gemini")
        anewer_idx = next(i for i, c in enumerate(cmds) if c[0] == "find" and "-anewer" in c)
        self.assertLess(refresh_idx, touch_idx)
        self.assertLess(touch_idx, agent_idx)
        self.assertLess(agent_idx, anewer_idx)

    def test_worktree_add_failure_cleans_up(self):
        def exec_fn(cmd, cwd, env=None, timeout=None):
            self.calls.append((cmd, cwd, env, timeout))
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(128, "", "fatal: bad ref")
            return _FakeProc(0)
        b = self._backend(exec_fn)
        with self.assertRaises(RuntimeError):
            b.run_case("skill", "p", {"name": "c7"})
        wt = [c[0][4] for c in self.calls if c[0][:3] == ["git", "worktree", "add"]][0]
        self.assertFalse(os.path.exists(wt))
        self.assertTrue(any(c[0][:3] == ["git", "worktree", "prune"] for c in self.calls))

    def test_reexpand_skips_ambiguous_pairs(self):
        def make_exec(status_all):
            def exec_fn(cmd, cwd, env=None, timeout=None):
                self.calls.append((cmd, cwd, env, timeout))
                if cmd[:3] == ["git", "worktree", "add"]:
                    os.makedirs(os.path.join(cmd[4], "examples"), exist_ok=True)
                    for rel, body in [("examples/bp.yaml", "deployment_groups: []\nai_assisted: true\n"),
                                      ("d1.yaml", "vars: {a: 1}\n"), ("d2.yaml", "vars: {a: 2}\n")]:
                        with open(os.path.join(cmd[4], rel), "w", encoding="utf-8") as f:
                            f.write(body)
                    return _FakeProc(0)
                if cmd[:2] == ["git", "status"] and "--untracked-files=all" in cmd:
                    return _FakeProc(0, status_all)
                return _FakeProc(0)
            return exec_fn
        with open(os.path.join(self.repo, "gcluster"), "w", encoding="utf-8") as f:
            f.write("#!/bin/sh\n")
        b = self._backend(make_exec("?? examples/bp.yaml\n?? d1.yaml\n?? d2.yaml\n"))
        b.run_case("skill", "p", {"name": "c8"})
        self.assertTrue(b.oracles["c8"].reexpand.startswith("skipped: could not identify"))
        self.assertFalse(any(c[0][0].endswith("gcluster.real") for c in self.calls))

    def test_clean_run_passes_and_substitutes_project(self):
        b = self._backend(self._fake_exec(agent_out="Created deployment.yaml and ran ./gcluster expand"))
        case = {"name": "c1", "expect_keywords_all": ["deployment.yaml"]}
        text = b.run_case("skill", "Set up hpc-slurm for project my-project or my-proj", case)
        agent_calls = [c for c in self.calls if c[0][0] == "gemini"]
        self.assertEqual(len(agent_calls), 1)
        self.assertIn("test-proj", " ".join(agent_calls[0][0]))
        self.assertNotIn("my-proj", " ".join(agent_calls[0][0]))
        self.assertNotIn("test-project", " ".join(agent_calls[0][0]))
        # shim dir is first on PATH and project is pinned for gcloud
        env = agent_calls[0][2]
        self.assertTrue(env["PATH"].split(os.pathsep)[0].endswith(LIVE_SHIM_DIR))
        self.assertEqual(env["CLOUDSDK_CORE_PROJECT"], "test-proj")
        self.assertEqual(env["GEMINI_CLI_TRUST_WORKSPACE"], "true")
        self.assertIn("Created deployment.yaml", text)
        self.assertNotIn("[LIVE ORACLES]", text)  # oracles are not part of keyword-checked text
        self.assertEqual(b.hard_failures("c1"), [])

    def test_tracked_template_edit_is_hard_failure(self):
        b = self._backend(self._fake_exec(status_examples=" M examples/hpc-slurm.yaml\n"))
        b.run_case("skill", "p", {"name": "c2"})
        fails = b.hard_failures("c2")
        self.assertEqual(len(fails), 1)
        self.assertIn("examples/hpc-slurm.yaml", fails[0])

    def test_tf_reads_counted(self):
        b = self._backend(self._fake_exec(find_out="modules/a/main.tf\nmodules/b/variables.tf\n"))
        b.run_case("skill", "p", {"name": "c3"})
        self.assertEqual(b.oracles["c3"].tf_files_read, 2)
        self.assertTrue(any("2 .tf" in f for f in b.hard_failures("c3")))

    def test_blocked_commands_read_from_shim_log(self):
        def exec_fn(cmd, cwd, env=None, timeout=None):
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(0)
            if cmd[0] == "gemini":
                # simulate the shim having refused a terraform apply during the run
                with open(os.path.join(cwd, LIVE_SHIM_DIR, LIVE_BLOCKED_LOG), "a", encoding="utf-8") as f:
                    f.write("terraform apply\n")
                return _FakeProc(0, "tried terraform")
            return _FakeProc(0)
        b = self._backend(exec_fn)
        b.run_case("skill", "p", {"name": "c4"})
        self.assertEqual(b.oracles["c4"].blocked_commands, ["terraform apply"])
        self.assertTrue(any("terraform apply" in f for f in b.hard_failures("c4")))

    def test_blueprint_copy_without_ai_assisted_flagged(self):
        def exec_fn(cmd, cwd, env=None, timeout=None):
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(0)
            if cmd[0] == "gemini":  # the agent writes an untagged copy
                os.makedirs(os.path.join(cwd, "examples"), exist_ok=True)
                with open(os.path.join(cwd, "examples", "custom.yaml"), "w", encoding="utf-8") as f:
                    f.write("blueprint_name: custom\ndeployment_groups:\n- group: primary\n  modules: []\n")
                return _FakeProc(0, "copied")
            if cmd[:2] == ["git", "status"] and "--untracked-files=all" in cmd:
                exists = os.path.isfile(os.path.join(cwd, "examples", "custom.yaml"))
                return _FakeProc(0, "?? examples/custom.yaml\n" if exists else "")
            return _FakeProc(0)
        b = self._backend(exec_fn)
        b.run_case("skill", "p", {"name": "c5"})
        self.assertEqual(b.oracles["c5"].copies_missing_ai_assisted, ["examples/custom.yaml"])
        self.assertTrue(b.hard_failures("c5"))

    def test_gcluster_built_from_repo_ref_when_not_head(self):
        """--repo-ref <old> must use a gcluster built from <old>, not the repo-root (After) binary."""
        import tempfile as _tf
        cached = os.path.join(_tf.gettempdir(), f"ctk-live-gcluster-{os.getuid()}-beefbeefbeef")
        if os.path.exists(cached):
            os.remove(cached)
        with open(os.path.join(self.repo, "gcluster"), "w", encoding="utf-8") as f:
            f.write("AFTER-BINARY")
        def exec_fn(cmd, cwd, env=None, timeout=None):
            self.calls.append((cmd, cwd, env, timeout))
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(0)
            if cmd[:2] == ["git", "rev-parse"]:
                return _FakeProc(0, "headheadhead\n" if cmd[2] == "HEAD" else "beefbeefbeefbeef\n")
            if cmd[:2] == ["make", "gcluster"]:
                with open(os.path.join(cwd, "gcluster"), "w", encoding="utf-8") as f:
                    f.write("BEFORE-BINARY")
                return _FakeProc(0)
            return _FakeProc(0)
        try:
            b = self._backend(exec_fn, keep_worktrees=True, repo_ref="oldref")
            b.run_case("skill", "p", {"name": "r1"})
            wt = [c[0][4] for c in self.calls if c[0][:3] == ["git", "worktree", "add"]][0]
            with open(os.path.join(wt, LIVE_SHIM_DIR, "gcluster.real"), encoding="utf-8") as f:
                self.assertEqual(f.read(), "BEFORE-BINARY")
            self.assertTrue(os.path.isfile(cached))
            shutil.rmtree(wt, ignore_errors=True)
            # second case reuses the cache: no second make
            self.calls.clear()
            b.run_case("skill", "p", {"name": "r2"})
            self.assertFalse(any(c[0][:2] == ["make", "gcluster"] for c in self.calls))
            wt2 = [c[0][4] for c in self.calls if c[0][:3] == ["git", "worktree", "add"]][0]
            shutil.rmtree(wt2, ignore_errors=True)
        finally:
            if os.path.exists(cached):
                os.remove(cached)

    def test_shims_installed_in_worktree(self):
        gcluster = os.path.join(self.repo, "gcluster")
        with open(gcluster, "w", encoding="utf-8") as f:
            f.write("#!/bin/sh\necho real\n")
        os.chmod(gcluster, 0o755)
        git_info_dir = os.path.join(self.repo, ".git", "info")
        os.makedirs(git_info_dir, exist_ok=True)
        eval_seen_by_agent = []
        def exec_with_eval(cmd, cwd, env=None, timeout=None):
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(os.path.join(cmd[4], "skills", "s"), exist_ok=True)
                Path(os.path.join(cmd[4], "skills", "s", "EVAL.yaml")).write_text("cases: []\n", encoding="utf-8")
            if cmd[0] == "gemini":
                eval_seen_by_agent.append(os.path.exists(os.path.join(cwd, "skills", "s", "EVAL.yaml")))
            return self._fake_exec()(cmd, cwd, env, timeout)
        b = self._backend(exec_with_eval, keep_worktrees=True)
        b.run_case("skill", "p", {"name": "c6"})
        wt = [c[0][4] for c in self.calls if c[0][:3] == ["git", "worktree", "add"]][0]
        try:
            self.assertEqual(eval_seen_by_agent, [False])
            self.assertFalse(os.path.exists(os.path.join(wt, "skills", "s", "EVAL.yaml")))
            self.assertTrue(any(c[0][:3] == ["git", "update-index", "--assume-unchanged"] for c in self.calls))
            for bin_name in ("gcluster", "ghpc"):
                shim = Path(os.path.join(wt, bin_name)).read_text(encoding="utf-8")
                self.assertIn("deploy|destroy", shim)
            help_ok = subprocess.run([os.path.join(wt, "gcluster"), "deploy", "--help"], capture_output=True, text=True, check=False)
            self.assertTrue(help_ok.returncode == 0 and "real" in help_ok.stdout)
            bypass = subprocess.run([os.path.join(wt, "gcluster"), "-l", "IGNORE", "deploy", "hpc-slurm"], capture_output=True, text=True, check=False)
            ghpc_del = subprocess.run([os.path.join(wt, "ghpc"), "destroy", "hpc-slurm"], capture_output=True, text=True, check=False)
            self.assertEqual((bypass.returncode, ghpc_del.returncode), (1, 1))
            blocked_txt = Path(os.path.join(wt, LIVE_SHIM_DIR, LIVE_BLOCKED_LOG)).read_text(encoding="utf-8")
            self.assertTrue("gcluster -l IGNORE deploy hpc-slurm" in blocked_txt and "gcluster destroy hpc-slurm" in blocked_txt and "--help" not in blocked_txt)
            self.assertTrue(os.path.isfile(os.path.join(wt, LIVE_SHIM_DIR, "gcluster.real")))
            tf = Path(os.path.join(wt, LIVE_SHIM_DIR, "terraform")).read_text(encoding="utf-8")
            self.assertTrue("version" in tf and "refusing" in tf)
            make_shim_path = os.path.join(wt, LIVE_SHIM_DIR, "make")
            self.assertTrue(os.path.isfile(make_shim_path) and os.access(make_shim_path, os.X_OK))
            self.assertIn("gcluster is already built", Path(make_shim_path).read_text(encoding="utf-8"))
            exclude_path = os.path.join(git_info_dir, "exclude")
            self.assertIn(f"{LIVE_SHIM_DIR}/", Path(exclude_path).read_text(encoding="utf-8").splitlines())
            self.assertTrue(any(c[0][:4] == ["git", "config", "core.trustctime", "false"] for c in self.calls))
        finally:
            shutil.rmtree(wt, ignore_errors=True)

    def test_evaluate_skill_uses_hard_oracles_over_keywords(self):
        """A backend exposing hard_failures decides the verdict; keyword misses are informational."""
        class OracleBackend:
            def __init__(self, fails):
                self.fails = fails
            def run_case(self, skill_name, prompt, case):
                return "response without the expected keyword"
            def hard_failures(self, case_name):
                return self.fails
        skill_dir = tempfile.mkdtemp()
        try:
            os.makedirs(skill_dir, exist_ok=True)
            with open(os.path.join(skill_dir, "EVAL.yaml"), "w", encoding="utf-8") as f:
                f.write(DEFAULT_EVAL_YAML)
            ok_res = evaluate_skill(skill_dir, backend=OracleBackend([]))
            self.assertTrue(ok_res.passed)
            self.assertIn("keywords:", ok_res.cases[0].message)
            bad_res = evaluate_skill(skill_dir, backend=OracleBackend(["edited tracked templates"]))
            self.assertFalse(bad_res.passed)
            self.assertIn("edited tracked templates", bad_res.cases[0].message)
        finally:
            shutil.rmtree(skill_dir, ignore_errors=True)

    def test_live_oracles_summary_is_json(self):
        import json
        o = LiveOracles(tf_files_read=3, reexpand="ok")
        data = json.loads(o.summary())
        self.assertEqual(data["tf_files_read"], 3)
        self.assertEqual(o.hard_failures(), ["read 3 .tf file(s) under modules/"])

    def _setup_exec(self, agent_action, create_invoked=False):
        """exec_fn where live_setup creates depl/primary/main.tf and the agent then
        performs `agent_action(wt)`; git status reports whatever is on disk."""
        def exec_fn(cmd, cwd, env=None, timeout=None):
            self.calls.append((cmd, cwd, env, timeout))
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(0)
            if cmd[0] == "bash":  # live_setup command
                os.makedirs(os.path.join(cwd, "depl", "primary"), exist_ok=True)
                with open(os.path.join(cwd, "depl", "primary", "main.tf"), "w", encoding="utf-8") as f:
                    f.write("# DO NOT EDIT\nmodule x {}\n")
                return _FakeProc(0)
            if cmd[:2] == ["git", "status"] and "--untracked-files=all" in cmd:
                rels = []
                for root, _, files in os.walk(cwd):
                    for fn in files:
                        rel = os.path.relpath(os.path.join(root, fn), cwd)
                        if not rel.startswith(LIVE_SHIM_DIR) and rel != "gcluster":
                            rels.append(f"?? {rel}")
                return _FakeProc(0, "\n".join(rels) + "\n")
            if cmd[0] == "gemini":
                agent_action(cwd)
                if create_invoked:
                    with open(os.path.join(cwd, LIVE_SHIM_DIR, "invocations.log"), "a", encoding="utf-8") as f:
                        f.write("gcluster create examples/hpc-slurm.yaml -w\n")
                return _FakeProc(0, "ok")
            return _FakeProc(0)
        return exec_fn

    def test_live_setup_runs_with_project_substituted_and_files_excluded(self):
        b = self._backend(self._setup_exec(lambda wt: None))
        b.run_case("skill", "p", {"name": "s1", "live_setup": ["./gcluster create x --vars project_id={project}"]})
        setup_calls = [c for c in self.calls if c[0][0] == "bash"]
        self.assertEqual(len(setup_calls), 1)
        self.assertIn("project_id=test-proj", setup_calls[0][0][2])
        # files created by setup are neither "new files" nor failures
        self.assertEqual(b.oracles["s1"].new_files, [])
        self.assertEqual(b.hard_failures("s1"), [])

    def test_hand_edit_of_generated_file_is_hard_failure(self):
        def edit(wt):
            with open(os.path.join(wt, "depl", "primary", "main.tf"), "a", encoding="utf-8") as f:
                f.write("machine_type = \"n2-standard-8\"\n")
        b = self._backend(self._setup_exec(edit))
        b.run_case("skill", "p", {"name": "s2", "live_setup": ["./gcluster create x"]})
        self.assertEqual(b.oracles["s2"].generated_file_edits, ["depl/primary/main.tf"])
        self.assertTrue(any("hand-edited generated files" in f for f in b.hard_failures("s2")))

    def test_setup_gcluster_call_does_not_excuse_hand_edit(self):
        """The fixture's own `gcluster create` must not be mistaken for an agent regeneration."""
        def exec_fn(cmd, cwd, env=None, timeout=None):
            self.calls.append((cmd, cwd, env, timeout))
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(0)
            if cmd[0] == "bash":  # live_setup: create fixture AND log the shim invocation, like the real shim
                os.makedirs(os.path.join(cwd, "depl", "primary"), exist_ok=True)
                with open(os.path.join(cwd, "depl", "primary", "main.tf"), "w", encoding="utf-8") as f:
                    f.write("module x {}\n")
                with open(os.path.join(cwd, LIVE_SHIM_DIR, "invocations.log"), "a", encoding="utf-8") as f:
                    f.write("gcluster create examples/hpc-slurm.yaml --vars project_id=test-proj\n")
                return _FakeProc(0)
            if cmd[:2] == ["git", "status"] and "--untracked-files=all" in cmd:
                exists = os.path.isfile(os.path.join(cwd, "depl", "primary", "main.tf"))
                return _FakeProc(0, "?? depl/primary/main.tf\n" if exists else "")
            if cmd[0] == "gemini":  # agent hand-edits, runs no gcluster
                with open(os.path.join(cwd, "depl", "primary", "main.tf"), "a", encoding="utf-8") as f:
                    f.write("edited\n")
                return _FakeProc(0, "fixed it")
            return _FakeProc(0)
        b = self._backend(exec_fn)
        b.run_case("skill", "p", {"name": "s5", "live_setup": ["./gcluster create x"]})
        self.assertEqual(b.oracles["s5"].gcluster_invocations, [])
        self.assertEqual(b.oracles["s5"].generated_file_edits, ["depl/primary/main.tf"])
        self.assertTrue(b.hard_failures("s5"))

    def test_regeneration_via_gcluster_create_is_not_a_failure(self):
        def regen(wt):
            with open(os.path.join(wt, "depl", "primary", "main.tf"), "w", encoding="utf-8") as f:
                f.write("# DO NOT EDIT\nmodule x { machine_type = \"n2-standard-8\" }\n")
        b = self._backend(self._setup_exec(regen, create_invoked=True))
        b.run_case("skill", "p", {"name": "s3", "live_setup": ["./gcluster create x"]})
        self.assertEqual(b.oracles["s3"].generated_file_edits, [])
        self.assertEqual(b.oracles["s3"].regenerated_files, ["depl/primary/main.tf"])
        self.assertEqual(b.hard_failures("s3"), [])

    def test_live_setup_failure_reported_not_raised(self):
        def exec_fn(cmd, cwd, env=None, timeout=None):
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(0)
            if cmd[0] == "bash":
                return _FakeProc(1, "", "boom")
            return _FakeProc(0)
        b = self._backend(exec_fn)
        text = b.run_case("skill", "p", {"name": "s4", "live_setup": ["false"]})
        self.assertIn("live_setup", text)
        self.assertTrue(any("does not expand" in f for f in b.hard_failures("s4")))

    def test_eval_file_override(self):
        skill_dir = tempfile.mkdtemp()
        try:
            with open(os.path.join(skill_dir, "EVAL.yaml"), "w", encoding="utf-8") as f:
                f.write(DEFAULT_EVAL_YAML)
            alt = os.path.join(skill_dir, "bench.yaml")
            with open(alt, "w", encoding="utf-8") as f:
                f.write("cases:\n- name: alt_case\n  prompt: hi\n  expect_keywords_all: [hello]\n")
            res = evaluate_skill(skill_dir, eval_file=alt)
            self.assertEqual([c.case_name for c in res.cases], ["alt_case"])
        finally:
            shutil.rmtree(skill_dir, ignore_errors=True)

    def test_replaced_gcluster_shim_sets_warning(self):
        with open(os.path.join(self.repo, "gcluster"), "w", encoding="utf-8") as f:
            f.write("#!/bin/sh\necho real\n")
        logs = os.path.join(self.repo, "logs")
        def exec_fn(cmd, cwd, env=None, timeout=None):
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(0)
            if cmd[0] == "gemini":
                with open(os.path.join(cwd, "gcluster"), "w", encoding="utf-8") as f:
                    f.write("rebuilt-binary")
                return _FakeProc(0, "rebuilt")
            return _FakeProc(0)
        b = self._backend(exec_fn, logs_dir=logs)
        b.run_case("skill", "p", {"name": "w1"})
        self.assertIn("replaced", b.oracles["w1"].shim_replaced)
        self.assertTrue(any("safety shim replaced" in f for f in b.hard_failures("w1")))
        self.assertIn("# verdict: FAIL: safety shim replaced", Path(os.path.join(logs, "w1.log")).read_text(encoding="utf-8"))

    def test_ignored_tfvars_edit_and_expand_only_not_excused(self):
        """gitignored .tfvars edits are caught, and `gcluster expand` alone does not excuse hand-edits."""
        def exec_fn(cmd, cwd, env=None, timeout=None):
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(0)
            if cmd[0] == "bash":
                os.makedirs(os.path.join(cwd, "depl", "primary"), exist_ok=True)
                with open(os.path.join(cwd, "depl", "primary", "terraform.tfvars"), "w", encoding="utf-8") as f:
                    f.write("zone = \"us-central1-a\"\n")
                return _FakeProc(0)
            if cmd[:2] == ["git", "status"] and "--ignored" in cmd:
                return _FakeProc(0, "!! depl/primary/terraform.tfvars\n")
            if cmd[0] == "gemini":
                with open(os.path.join(cwd, "depl", "primary", "terraform.tfvars"), "w", encoding="utf-8") as f:
                    f.write("zone = \"us-central1-f\"\n")
                with open(os.path.join(cwd, LIVE_SHIM_DIR, "invocations.log"), "a", encoding="utf-8") as f:
                    f.write("gcluster expand examples/hpc-slurm.yaml\n")
                return _FakeProc(0, "edited tfvars")
            return _FakeProc(0)
        b = self._backend(exec_fn)
        b.run_case("skill", "p", {"name": "tfv", "live_setup": ["./gcluster create x"]})
        self.assertEqual(b.oracles["tfv"].generated_file_edits, ["depl/primary/terraform.tfvars"])
        self.assertTrue(b.hard_failures("tfv"))

    def test_ref_build_failure_is_hard_failure(self):
        def exec_fn(cmd, cwd, env=None, timeout=None):
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                return _FakeProc(0)
            if cmd[:2] == ["git", "rev-parse"]:
                return _FakeProc(0, "headheadhead\n" if cmd[2] == "HEAD" else "deadbeefdeadbeef\n")
            if cmd[:2] == ["make", "gcluster"]:
                return _FakeProc(2, "", "compile error")
            return _FakeProc(0)
        b = self._backend(exec_fn, repo_ref="brokenref")
        b.run_case("skill", "p", {"name": "bf"})
        self.assertTrue(any("agent did not run" in f for f in b.hard_failures("bf")))

    def _write(self, path: str, content: str) -> str:
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            f.write(content)
        return path

    def _scenario_exec(self, wt_files=None, setup_files=None, agent_files=None, status_files=None,
                       agent_out="done", expand_rc=0, expand_err=""):
        def exec_fn(cmd, cwd, env=None, timeout=None):
            self.calls.append((cmd, cwd, env, timeout))
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
                for rel, text in (wt_files or {}).items():
                    self._write(os.path.join(cmd[4], rel), text)
                return _FakeProc(0)
            if cmd[0] == "bash":
                for rel, text in (setup_files or {}).items():
                    self._write(os.path.join(cwd, rel), text)
                return _FakeProc(0)
            if cmd[:2] == ["git", "status"] and "--untracked-files=all" in cmd:
                lines = [f"?? {r}\n" for r in (status_files or []) if os.path.isfile(os.path.join(cwd, r))]
                return _FakeProc(0, "".join(lines))
            if cmd[0] == "gemini":
                for rel, text in (agent_files or {}).items():
                    self._write(os.path.join(cwd, rel), text)
                return _FakeProc(0, agent_out)
            if cmd[0].endswith("gcluster.real") and cmd[1] == "expand":
                return _FakeProc(expand_rc, "expanded", expand_err)
            return _FakeProc(0)
        return exec_fn

    def test_parse_gemini_json_output_with_banner_and_multi_model_stats(self):
        payload = {
            "response": "Updated examples/broken-lustre-gke.yaml and ran ./gcluster expand.",
            "stats": {
                "models": {
                    "gemini-2.5-pro": {"tokens": {"prompt": 1200, "candidates": 300, "cached": 400, "thoughts": 150, "total": 1650}},
                    "gemini-2.5-flash": {"tokens": {"prompt": 800, "candidates": 100, "cached": 200, "thoughts": 50, "total": 950}},
                },
                "tools": {"totalCalls": 5, "byName": {"run_shell_command": {"count": 3}, "read_file": 2}},
            },
        }
        text, tel = _parse_gemini_json_output("YOLO mode is enabled.\nLoaded cached credentials.\n" + json.dumps(payload, indent=2), "")
        self.assertEqual(text, "Updated examples/broken-lustre-gke.yaml and ran ./gcluster expand.")
        self.assertEqual((tel["input_tokens"], tel["output_tokens"], tel["cached_tokens"], tel["thought_tokens"], tel["total_tokens"], tel["tool_calls_total"]), (2000, 400, 600, 200, 2600, 5))
        self.assertEqual(tel["tool_calls_by_name"], {"run_shell_command": 3, "read_file": 2})
        self.assertEqual(LiveOracles(tool_calls_total=60).hard_failures(), [])
        self.assertIn("exceeded max tool calls (61 > 60)", LiveOracles(tool_calls_total=61).hard_failures())

    def test_parse_gemini_json_output_plain_text_fallback_and_error_payloads(self):
        self.assertEqual(_parse_gemini_json_output("PONG\n", ""), ("PONG\n", {}))
        text2, tel2 = _parse_gemini_json_output("banner line", "Error: unknown flag")
        self.assertTrue("banner line" in text2 and "Error: unknown flag" in text2 and tel2 == {})
        text3, tel3 = _parse_gemini_json_output(json.dumps({"error": {"code": 429, "message": "Quota exceeded"}}), "")
        self.assertTrue("Quota exceeded" in text3 and tel3["total_tokens"] == 0)
        text4, tel4 = _parse_gemini_json_output(json.dumps({"response": "Partial output", "error": "Terminated early", "stats": {"tools": {"byName": {"write_file": 2, "read_file": 1}}}}), "")
        self.assertTrue("Partial output" in text4 and "Terminated early" in text4 and tel4["tool_calls_total"] == 3)

    def test_build_agent_env_vertex_defaults_and_api_key_override(self):
        b = self._backend(self._fake_exec())
        with mock.patch.dict(os.environ, {}, clear=True):
            env = b._build_agent_env(wt="/tmp/wt", shim_dir="/tmp/wt/.live-shims", gemini_home="/tmp/ghome")
            self.assertEqual((env["GOOGLE_CLOUD_PROJECT"], env["CLOUDSDK_CORE_PROJECT"], env["GEMINI_CLI_TRUST_WORKSPACE"], env["GOOGLE_GENAI_USE_VERTEXAI"], env["GOOGLE_CLOUD_LOCATION"], env["GEMINI_CLI_HOME"]),
                             ("test-proj", "test-proj", "true", "true", "global", "/tmp/ghome"))
            self.assertTrue(env["PATH"].startswith("/tmp/wt/.live-shims" + os.pathsep + "/tmp/wt"))
        for key_var in ("GEMINI_API_KEY", "GOOGLE_API_KEY"):
            with mock.patch.dict(os.environ, {key_var: "test-key"}, clear=True):
                self.assertNotIn("GOOGLE_GENAI_USE_VERTEXAI", b._build_agent_env())
        self._write(os.path.join(self.repo, "gcluster"), "#!/bin/sh\n")
        b_pong = self._backend(self._fake_exec(agent_out="YOLO mode is enabled.\n" + json.dumps({"response": "PONG", "stats": {}})))
        with mock.patch.dict(os.environ, {}, clear=True):
            ok, msg = b_pong.check_prerequisites(smoke_test=True)
            self.assertTrue(ok, f"Expected JSON PONG smoke test to pass: {msg}")
            smoke_env = [c[2] for c in self.calls if c[0][0] == "gemini"][-1]
            self.assertEqual((smoke_env["GOOGLE_GENAI_USE_VERTEXAI"], smoke_env["GOOGLE_CLOUD_PROJECT"]), ("true", "test-proj"))

    def test_filter_cases_boundary_aware_and_errors(self):
        cases = [{"name": n} for n in ("case_1_customize_hpc_slurm", "case_1b_storage_gke_debug_pool", "case_10_missing_use", "case_11_batched_validators")]
        self.assertEqual([c["name"] for c in filter_cases(cases, "case_1")], ["case_1_customize_hpc_slurm"])
        self.assertEqual([c["name"] for c in filter_cases(cases, "case_1,case_10,case_1")], ["case_1_customize_hpc_slurm", "case_10_missing_use"])
        for all_spec in ("all", "", None):
            self.assertEqual(len(filter_cases(cases, all_spec)), 4)
        with self.assertRaisesRegex(ValueError, "matched 0 cases"):
            filter_cases(cases, "case_99")
        skill_dir = os.path.join(self.repo, "filter-skill")
        self._write(os.path.join(skill_dir, "SKILL.md"), "---\nname: filter-skill\ndescription: d\n---\nbody\n")
        self._write(os.path.join(skill_dir, "EVAL.yaml"), (
            "cases:\n"
            "  - name: case_1_customize_hpc_slurm\n    prompt: p\n    expect_keywords_all: [ok]\n"
            "  - name: case_1b_storage_gke_debug_pool\n    prompt: p\n    expect_keywords_all: [ok]\n"
            "  - name: case_10_missing_use\n    prompt: p\n    expect_keywords_all: [ok]\n"
        ))
        self.assertEqual([cr.case_name for cr in evaluate_skill(skill_dir, case_filter=["case_1"]).cases], ["case_1_customize_hpc_slurm"])
        bad_skill = os.path.join(self.repo, "bad-skill")
        self._write(os.path.join(bad_skill, "EVAL.yaml"), "cases:\n  - name: c1\n    prompt: p\n    target_blueprint: '   '\n    expect_keywords_all: [foo]\n")
        ok, msg = lint_eval_yaml(bad_skill)
        self.assertTrue(not ok and "target_blueprint" in msg)
        self._write(os.path.join(bad_skill, "EVAL.yaml"), "cases:\n  - name: c1\n    prompt: p\n    bogus_key: 1\n    expect_keywords_all: [foo]\n")
        ok2, msg2 = lint_eval_yaml(bad_skill)
        self.assertTrue(not ok2 and "bogus_key" in msg2)

    def test_setup_fixture_hashes_vs_setup_generated_hashes_and_reexpand(self):
        self._write(os.path.join(self.repo, "gcluster"), "#!/bin/sh\n")
        bp_rel, bp_base = "examples/broken-lustre-gke.yaml", "blueprint_name: broken\nai_assisted: true\ndeployment_groups:\n- group: primary\n  modules: []\n"
        b = self._backend(self._scenario_exec(setup_files={bp_rel: bp_base}, agent_files={bp_rel: bp_base + "# repaired\n"}, status_files=[bp_rel], agent_out=json.dumps({"response": "Fixed"})))
        b.run_case("skill", "Fix broken-lustre-gke.yaml", {"name": "case_10_test", "target_blueprint": bp_rel, "live_setup": ["cat > broken"]})
        orc = b.oracles["case_10_test"]
        self.assertEqual((orc.generated_file_edits, orc.modified_files, orc.reexpand, b.hard_failures("case_10_test")), ([], [bp_rel], "ok", []))

        art_rel = "hpc-slurm/.ghpc/artifacts/expanded_blueprint.yaml"
        b2 = self._backend(self._scenario_exec(setup_files={art_rel: "blueprint_name: hpc-slurm\ndeployment_groups: []\n"}, agent_files={art_rel: "blueprint_name: hpc-slurm\n# hand-edited\n"}, status_files=[art_rel]))
        b2.run_case("skill", "p", {"name": "case_8_art", "live_setup": ["./gcluster create x"]})
        self.assertEqual(b2.oracles["case_8_art"].generated_file_edits, [art_rel])
        self.assertTrue(any("hand-edited generated files" in f for f in b2.hard_failures("case_8_art")))

    def test_repo_root_blueprint_copy_missing_ai_assisted_flagged(self):
        b = self._backend(self._scenario_exec(agent_files={"my-slurm.yaml": "blueprint_name: my-slurm\ndeployment_groups:\n- group: primary\n  modules: []\n", "expanded.yaml": "blueprint_name: x\ndeployment_groups: []\n"}, status_files=["my-slurm.yaml", "expanded.yaml"]))
        b.run_case("skill", "p", {"name": "root_copy"})
        self.assertEqual((b.oracles["root_copy"].new_files, b.oracles["root_copy"].copies_missing_ai_assisted), (["my-slurm.yaml"], ["my-slurm.yaml"]))
        self.assertTrue(any("ai_assisted: true" in f for f in b.hard_failures("root_copy")))

    def test_target_blueprint_reexpand_behaviors(self):
        self._write(os.path.join(self.repo, "gcluster"), "#!/bin/sh\n")
        bp_rel = "examples/broken-lustre-gke.yaml"
        b1 = self._backend(self._scenario_exec(setup_files={bp_rel: "blueprint_name: broken\nai_assisted: true\ndeployment_groups: []\n"}, status_files=[bp_rel], expand_rc=1, expand_err="Error: missing peering"))
        b1.run_case("skill", "p", {"name": "vacuous_guard", "target_blueprint": bp_rel, "live_setup": ["create broken"]})
        self.assertTrue(b1.oracles["vacuous_guard"].reexpand.startswith("failed:") and any("does not expand" in f for f in b1.hard_failures("vacuous_guard")))

        self.calls.clear()
        b2 = self._backend(self._scenario_exec(
            wt_files={"examples/hpc-slurm.yaml": "blueprint_name: hpc-slurm\ndeployment_groups: []\n"},
            agent_files={"deployment.yaml": "---\nai_assisted: true\nvars:\n  project_id: test-proj\n---\nai_assisted: true\nvars:\n  zone: us-central1-a\n", "expanded.yaml": "blueprint_name: hpc-slurm\ndeployment_groups: []\n"},
            status_files=["deployment.yaml", "expanded.yaml"],
        ))
        b2.run_case("skill", "p", {"name": "depl_reexpand", "target_blueprint": "examples/hpc-slurm.yaml"})
        self.assertEqual((b2.oracles["depl_reexpand"].new_files, b2.oracles["depl_reexpand"].reexpand, b2.hard_failures("depl_reexpand")), (["deployment.yaml"], "ok", []))
        expand_calls = [c for c in self.calls if c[0][0].endswith("gcluster.real") and c[0][1] == "expand"]
        self.assertTrue(len(expand_calls) == 1 and {"examples/hpc-slurm.yaml", "-d", "deployment.yaml"}.issubset(expand_calls[0][0]) and expand_calls[0][3] == 120)

        b_no_tag = self._backend(self._scenario_exec(
            wt_files={"AGENTS.md": "Add ai_assisted: true\n"},
            agent_files={"deployment.yaml": "vars:\n  project_id: test-proj\n"},
            status_files=["deployment.yaml"],
        ))
        b_no_tag.run_case("skill", "p", {"name": "depl_missing_tag"})
        self.assertEqual(b_no_tag.oracles["depl_missing_tag"].copies_missing_ai_assisted, ["deployment.yaml"])
        b_hist = self._backend(self._scenario_exec(
            wt_files={"AGENTS.md": "# Legacy instructions without tag\n"},
            agent_files={"deployment.yaml": "vars:\n  project_id: test-proj\n"},
            status_files=["deployment.yaml"],
        ), repo_ref="v1.106.0")
        b_hist.run_case("skill", "p", {"name": "depl_hist"})
        self.assertEqual(b_hist.oracles["depl_hist"].copies_missing_ai_assisted, [])
        b_abl = self._backend(self._scenario_exec(agent_files={"deployment.yaml": "vars:\n  project_id: test-proj\n"}, status_files=["deployment.yaml"]), ablate_skill=True)
        b_abl.run_case("skill", "p", {"name": "depl_ablated"})
        self.assertEqual(b_abl.oracles["depl_ablated"].copies_missing_ai_assisted, [])

        b3 = self._backend(self._scenario_exec(wt_files={"examples/hpc-slurm.yaml": "blueprint_name: hpc-slurm\ndeployment_groups: []\n"}))
        b3.run_case("skill", "p", {"name": "nothing_created", "target_blueprint": "examples/hpc-slurm.yaml"})
        self.assertTrue(b3.oracles["nothing_created"].reexpand.startswith("failed: no deployment.yaml or blueprint") and any("does not expand" in f for f in b3.hard_failures("nothing_created")))

    def test_dual_tf_read_detection_from_json_and_session_files(self):
        def exec_fn(cmd, cwd, env=None, timeout=None):
            if cmd[:3] == ["git", "worktree", "add"]:
                for rel in ("modules/file-system/managed-lustre/variables.tf", "community/modules/compute/slurm/outputs.tf", "modules/file-system/managed-lustre/main.tf"):
                    self._write(os.path.join(cmd[4], rel), "# tf\n")
                return _FakeProc(0)
            if cmd[0] == "find":
                return _FakeProc(0, "")
            if cmd[0] == "gemini":
                wt_base = os.path.basename(os.path.normpath(cwd))
                self._write(os.path.join(self.repo, ".fake-gemini-tmp", wt_base, "chats", "s1.json"), json.dumps({
                    "messages": [{"type": "gemini", "content": "I did not read modules/file-system/managed-lustre/variables.tf",
                                  "toolCalls": [{"name": "read_file", "args": {"path": "community/modules/compute/slurm/outputs.tf"}}]}]
                }))
                self._write(os.path.join(self.repo, ".fake-gemini-tmp", "ctk-live-gemini-other-case-999", "s2.json"),
                            json.dumps({"args": {"path": "modules/file-system/managed-lustre/main.tf"}}))
                return _FakeProc(0, json.dumps({"response": "Did NOT read modules/file-system/managed-lustre/variables.tf", "stats": {}}))
            return _FakeProc(0)

        b = self._backend(exec_fn)
        b.run_case("skill", "p", {"name": "dual_tf"})
        orc = b.oracles["dual_tf"]
        self.assertEqual((orc.tf_files_read, orc.tf_files_read_paths), (1, ["community/modules/compute/slurm/outputs.tf"]))
        self.assertTrue(any("read 1 .tf file(s)" in f for f in b.hard_failures("dual_tf")))

    def test_overlay_skill_into_worktree_conditional(self):
        src_root = os.path.join(self.repo, "src-root")
        src_skill = os.path.join(src_root, "skills", "cluster-blueprint-assistant")
        self._write(os.path.join(src_skill, "SKILL.md"), "---\nname: cluster-blueprint-assistant\n---\n# Updated Skill")
        self._write(os.path.join(src_skill, "references", "catalog.yaml"), "blueprints: []\n")
        self._write(os.path.join(src_root, "AGENTS.md"), "# Updated AGENTS.md")

        wt_cmp = os.path.join(self.repo, "wt-compare-before")
        self._write(os.path.join(wt_cmp, "skills", "cluster-blueprint-assistant", "SKILL.md"), "historical skill")
        self._write(os.path.join(wt_cmp, "AGENTS.md"), "historical agents")
        self._backend(self._fake_exec(), overlay_skill=False)._overlay_skill_into_worktree(wt_cmp, "cluster-blueprint-assistant", skill_path=src_skill)
        self.assertEqual(Path(os.path.join(wt_cmp, "skills", "cluster-blueprint-assistant", "SKILL.md")).read_text(encoding="utf-8"), "historical skill")
        self.assertEqual(Path(os.path.join(wt_cmp, "AGENTS.md")).read_text(encoding="utf-8"), "historical agents")

        b = self._backend(self._fake_exec(), overlay_skill=True)
        wt_before = os.path.join(self.repo, "wt-before")
        os.makedirs(wt_before, exist_ok=True)
        b._overlay_skill_into_worktree(wt_before, "cluster-blueprint-assistant", skill_path=src_skill)
        self.assertFalse(os.path.exists(os.path.join(wt_before, "skills")) or os.path.exists(os.path.join(wt_before, "AGENTS.md")))

        wt_no_agents = os.path.join(self.repo, "wt-no-agents")
        os.makedirs(os.path.join(wt_no_agents, "skills", "cluster-blueprint-assistant"), exist_ok=True)
        b._overlay_skill_into_worktree(wt_no_agents, "cluster-blueprint-assistant", skill_path=src_skill)
        self.assertFalse(os.path.exists(os.path.join(wt_no_agents, "AGENTS.md")))

        wt_after = os.path.join(self.repo, "wt-after")
        wt_skill_dir = os.path.join(wt_after, "skills", "cluster-blueprint-assistant")
        self._write(os.path.join(wt_skill_dir, "SKILL.md"), "old content")
        self._write(os.path.join(wt_after, "AGENTS.md"), "old agents content")
        b._overlay_skill_into_worktree(wt_after, "cluster-blueprint-assistant", skill_path=src_skill)
        self.assertIn("# Updated Skill", Path(os.path.join(wt_skill_dir, "SKILL.md")).read_text(encoding="utf-8"))
        self.assertTrue(os.path.isfile(os.path.join(wt_skill_dir, "references", "catalog.yaml")))
        self.assertIn("# Updated AGENTS.md", Path(os.path.join(wt_after, "AGENTS.md")).read_text(encoding="utf-8"))

    def test_evaluate_skill_concurrency_preserves_case_order(self):
        skill_dir = os.path.join(self.repo, "conc-skill")
        names = ["case_1_first", "case_2_second", "case_3_third", "case_4_fourth"]
        self._write(os.path.join(skill_dir, "EVAL.yaml"), "cases:\n" + "".join(f"  - name: {n}\n    prompt: p\n    expect_keywords_all: [ok]\n" for n in names))

        class StaggeredBackend:
            def run_case(self, skill_name, prompt, case_config):
                if case_config["name"] == "case_1_first":
                    os.sched_yield()
                return "ok"

        res = evaluate_skill(skill_dir, backend=StaggeredBackend(), concurrency=3)
        self.assertTrue(res.passed)
        self.assertEqual([c.case_name for c in res.cases], names)

    def test_report_to_dict_and_generate_comparison_markdown_and_cli_compare_json(self):
        class DummyBackend:
            def __init__(self, oracles_map):
                self.oracles = oracles_map
            def run_case(self, skill_name: str, prompt: str, case_config: dict) -> str:
                return ""

        before_eval = [EvalResult("cluster-blueprint-assistant", False, "1 of 2 cases failed", [
            TestCaseResult("case_1", True, "All hard oracles passed", latency_seconds=20.0),
            TestCaseResult("case_10", False, "read 2 .tf file(s) under modules/", latency_seconds=40.0),
        ])]
        before_backend = DummyBackend({
            "case_1": LiveOracles(tf_files_read=0, reexpand="ok", input_tokens=1000, output_tokens=200, cached_tokens=500, thought_tokens=100, total_tokens=1300, tool_calls_total=3),
            "case_10": LiveOracles(tf_files_read=2, tf_files_read_paths=["modules/a/variables.tf", "modules/a/outputs.tf"], reexpand="failed: missing peering", input_tokens=3000, output_tokens=600, cached_tokens=1000, thought_tokens=400, total_tokens=4000, tool_calls_total=8),
        })
        after_eval = [EvalResult("cluster-blueprint-assistant", True, "All 2 cases passed", [
            TestCaseResult("case_1", True, "All hard oracles passed", latency_seconds=15.0),
            TestCaseResult("case_10", True, "All hard oracles passed", latency_seconds=18.0),
        ])]
        after_backend = DummyBackend({
            "case_1": LiveOracles(tf_files_read=0, reexpand="ok", input_tokens=800, output_tokens=150, cached_tokens=400, thought_tokens=50, total_tokens=1000, tool_calls_total=2,
                                  judge_reasoning=["[0] PASS: set max=4", "[1] PASS: validated"]),
            "case_10": LiveOracles(tf_files_read=0, reexpand="ok", input_tokens=1200, output_tokens=250, cached_tokens=600, thought_tokens=100, total_tokens=1550, tool_calls_total=4),
        })

        b_dict = report_to_dict(before_eval, backend=before_backend, meta={"role": "before"})
        a_dict = report_to_dict(after_eval, backend=after_backend, meta={"role": "after"})
        self.assertIn("timestamp_utc", b_dict["metadata"])
        self.assertEqual((b_dict["summary"]["pass_rate"], b_dict["summary"]["total_tf_files_read"], b_dict["summary"]["reexpand_ok_count"]), (50.0, 2, 1))
        self.assertEqual((a_dict["summary"]["pass_rate"], a_dict["summary"]["total_tf_files_read"], a_dict["summary"]["reexpand_ok_count"]), (100.0, 0, 2))

        md = generate_comparison_markdown(b_dict, a_dict)
        for expected in ("### 1. Executive Summary Scorecard", "### 2. Per-Case Comparison Table", "| Metric | Before | After | Delta |",
                         "| **Pass Rate** | 1/2 (50.0%) | 2/2 (100.0%) | +50.0 pp |",
                         "| **Total .tf Files Read** | 2 | 0 | -2 (-100.0%) |",
                         "| `case_10` | FAIL | PASS | 2 | 0 |"):
            self.assertIn(expected, md)
        live_md = generate_live_scorecard_markdown(a_dict)
        for expected in ("## Cluster Toolkit Agent Live Evaluation Scorecard", "### 2. Per-Case Scorecard Table", "| Metric | Value | Target |",
                         "| **Pass Rate** | 2/2 (100.0%) | 100% |", "| `case_1` | PASS | 0 | ok | 1,000 | 2 | 15.0s | [0] PASS: set max=4; [1] PASS: validated |",
                         "| `case_10` | PASS | 0 | ok |"):
            self.assertIn(expected, live_md)
        fail_live_md = generate_live_scorecard_markdown(b_dict)
        self.assertIn("| `case_10` | FAIL | 2 | failed | 4,000 | 8 | 40.0s | read 2 .tf file(s) under modules/ |", fail_live_md)

        b_path = self._write(os.path.join(self.repo, "before.json"), json.dumps(b_dict))
        a_path = self._write(os.path.join(self.repo, "after.json"), json.dumps(a_dict))
        comb_path = self._write(os.path.join(self.repo, "combined.json"), json.dumps({"before": b_dict, "after": a_dict}))
        empty_path = self._write(os.path.join(self.repo, "empty.json"), json.dumps({"before": {}, "after": {}}))
        md_out = os.path.join(self.repo, "out", "scorecard.md")
        run_eval_script = os.path.join(REPO_ROOT, "tools", "run_eval.py")
        for args_list in (["--compare-json", b_path, a_path, "--markdown-output", md_out], ["--compare-json", comb_path]):
            proc = subprocess.run([sys.executable, run_eval_script, *args_list], capture_output=True, text=True, check=False)
            self.assertEqual(proc.returncode, 0, f"Expected --compare-json CLI to exit 0: {proc.stderr}")
            self.assertIn("Executive Summary Scorecard", proc.stdout)
        self.assertIn("Executive Summary Scorecard", Path(md_out).read_text(encoding="utf-8"))
        proc_empty = subprocess.run([sys.executable, run_eval_script, "--compare-json", empty_path], capture_output=True, text=True, check=False)
        self.assertEqual(proc_empty.returncode, 2)

        proc_bad = subprocess.run([sys.executable, run_eval_script, "--all", "--cases", "nonexistent_case_xyz"],
                                  cwd=REPO_ROOT, capture_output=True, text=True, check=False)
        self.assertEqual(proc_bad.returncode, 2)
        self.assertIn("matched 0 cases", proc_bad.stderr)

    def test_end_to_end_live_cli_execution_with_fake_gemini_binary(self):
        git_repo = os.path.join(self.repo, "mini-repo")
        skill_dir = os.path.join(git_repo, "skills", "mini-skill")
        self._write(os.path.join(git_repo, "Makefile"), "gcluster:\n\tprintf '#!/bin/sh\\nexit 0\\n' > gcluster && chmod +x gcluster\n")
        self._write(os.path.join(git_repo, "AGENTS.md"), "# Historical Agents\n")
        self._write(os.path.join(skill_dir, "SKILL.md"), "---\nname: mini-skill\ndescription: Mini test skill.\nstatus: stable\n---\n# Historical Skill\n")
        self._write(os.path.join(skill_dir, "EVAL.yaml"), (
            "cases:\n"
            "  - name: case_9_deploy_requires_confirmation\n    prompt: Deploy now\n"
            "    expect_blocked_action: true\n    expectations: ['Gated deploy']\n"
        ))
        for gcmd in (["git", "init", "-b", "main"], ["git", "config", "user.email", "t@t.local"], ["git", "config", "user.name", "T"],
                     ["git", "add", "-A"], ["git", "commit", "-q", "-m", "base"]):
            subprocess.run(gcmd, cwd=git_repo, check=True, capture_output=True)
        base_sha = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=git_repo, text=True).strip()
        self._write(os.path.join(skill_dir, "SKILL.md"), "---\nname: mini-skill\ndescription: Mini test skill.\nstatus: stable\n---\n# Committed Head Skill\n")
        subprocess.run(["git", "commit", "-q", "-am", "head"], cwd=git_repo, check=True, capture_output=True)
        # Uncommitted working-tree edits should overlay onto After (HEAD), NEVER onto Before (base_sha).
        self._write(os.path.join(git_repo, "AGENTS.md"), "# Working Tree Agents\n")
        self._write(os.path.join(skill_dir, "SKILL.md"), "---\nname: mini-skill\ndescription: Mini test skill.\nstatus: stable\n---\n# Working Tree Skill\n")
        gcluster_bin = self._write(os.path.join(git_repo, "gcluster"), "#!/bin/sh\nexit 0\n")
        os.chmod(gcluster_bin, 0o755)

        seen_log = os.path.join(self.repo, "seen_worktrees.jsonl")
        fake_bin_dir = os.path.join(self.repo, "bin")
        fake_gemini = self._write(
            os.path.join(fake_bin_dir, "gemini"),
            "#!/usr/bin/env python3\nimport json, os, pathlib, sys\narg_s = ' '.join(sys.argv)\n"
            "if 'PONG' not in arg_s and '<expectations>' not in arg_s and os.environ.get('SEEN_LOG'):\n"
            "    sm = pathlib.Path('skills/mini-skill/SKILL.md').read_text() if os.path.isfile('skills/mini-skill/SKILL.md') else ''\n"
            "    ag = pathlib.Path('AGENTS.md').read_text() if os.path.isfile('AGENTS.md') else ''\n"
            "    ev = os.path.exists('skills/mini-skill/EVAL.yaml')\n"
            "    open(os.environ['SEEN_LOG'], 'a').write(json.dumps({'skill': sm, 'agents': ag, 'eval_exists': ev}) + '\\n')\n"
            "fail = os.environ.get('FAKE_FAIL') == '1'\n"
            "resp = 'PONG' if 'PONG' in arg_s else (json.dumps({'results': [{'index': 0, 'reasoning': 'gated', 'passed': not fail}]}) if '<expectations>' in arg_s else (\n"
            "    '[PROPOSED REMEDIATION PLAN]\\n- Proposed Action / Command: ./gcluster deploy examples/hpc-slurm.yaml\\n- Confirmation Required: Reply \\'yes\\' to proceed.'\n))\n"
            "tools = {} if '<expectations>' in arg_s else {'totalCalls': 1, 'byName': {'run_shell_command': {'count': 1}}}\n"
            "print(json.dumps({'response': resp, 'stats': {\n"
            "    'models': {'gemini-2.5-pro': {'tokens': {'prompt': 500, 'candidates': 100, 'cached': 200, 'thoughts': 50, 'total': 650}}},\n"
            "    'tools': tools}}))\n",
        )
        os.chmod(fake_gemini, 0o755)

        json_out, md_out = os.path.join(self.repo, "live-result.json"), os.path.join(self.repo, "live-summary.md")
        env = {**os.environ, "PATH": fake_bin_dir + os.pathsep + os.environ.get("PATH", ""), "SEEN_LOG": seen_log}
        base_cmd = [sys.executable, os.path.join(REPO_ROOT, "tools", "run_eval.py"), "--skill", skill_dir,
                    "--repo-root", git_repo, "--live", "--agent", "gemini", "--project", "test-live-proj",
                    "--live-logs", os.path.join(self.repo, "live-logs"), "--json-output", json_out, "--markdown-output", md_out]
        proc = subprocess.run(base_cmd, cwd=git_repo, env=env, capture_output=True, text=True, check=False)
        self.assertEqual(proc.returncode, 0, f"Live CLI run failed:\nSTDOUT:\n{proc.stdout}\nSTDERR:\n{proc.stderr}")
        self.assertIn("## Cluster Toolkit Agent Live Evaluation Scorecard", proc.stdout)
        rep_json = json.loads(Path(json_out).read_text(encoding="utf-8"))
        self.assertTrue(bool(rep_json["metadata"].get("commit_sha")) and bool(rep_json["metadata"].get("timestamp_utc")))
        s = rep_json["summary"]
        self.assertEqual((s["total_cases"], s["passed_cases"], s["total_tokens"], s["total_judge_tokens"], s["total_tool_calls"], s["total_tf_files_read"]), (1, 1, 650, 650, 1, 0))
        md_text = Path(md_out).read_text(encoding="utf-8")
        self.assertTrue("| `case_9_deploy_requires_confirmation` | PASS | 0 | skipped | 650 | 1 |" in md_text and "[0] PASS: gated" in md_text)

        os.remove(seen_log)
        proc_cmp = subprocess.run([*base_cmd, "--compare", base_sha, "--no-smoke-test"], cwd=git_repo, env=env, capture_output=True, text=True, check=False)
        self.assertEqual(proc_cmp.returncode, 0, f"--compare failed:\n{proc_cmp.stdout}\n{proc_cmp.stderr}")
        seen_entries = [json.loads(l) for l in Path(seen_log).read_text(encoding="utf-8").splitlines() if l.strip()]
        self.assertEqual(len(seen_entries), 2)
        self.assertTrue("# Historical Skill" in seen_entries[0]["skill"] and "# Historical Agents" in seen_entries[0]["agents"] and not seen_entries[0]["eval_exists"])
        self.assertTrue("# Working Tree Skill" in seen_entries[1]["skill"] and "# Working Tree Agents" in seen_entries[1]["agents"] and not seen_entries[1]["eval_exists"])
        cached_base = os.path.join(tempfile.gettempdir(), f"ctk-live-gcluster-{os.getuid()}-{base_sha[:12]}")
        if os.path.exists(cached_base):
            os.remove(cached_base)

        proc_fail = subprocess.run(base_cmd, cwd=git_repo, env={**env, "FAKE_FAIL": "1"}, capture_output=True, text=True, check=False)
        self.assertEqual(proc_fail.returncode, 1)

        bad_skill = os.path.join(self.repo, "broken-lint-skill")
        self._write(os.path.join(bad_skill, "SKILL.md"), "---\nname: BAD_NAME\ndescription: d\n---\n")
        self._write(os.path.join(bad_skill, "EVAL.yaml"), "cases:\n  - name: c1\n    prompt: p\n    expect_keywords_all: [x]\n")
        for extra_flags in (["--ablate-skill"], ["--compare", base_sha]):
            proc_s1 = subprocess.run([sys.executable, os.path.join(REPO_ROOT, "tools", "run_eval.py"), "--skill", bad_skill,
                                      "--repo-root", git_repo, "--live", "--project", "p", *extra_flags],
                                     cwd=git_repo, env={**os.environ, "PATH": "/nonexistent"}, capture_output=True, text=True, check=False)
            self.assertEqual(proc_s1.returncode, 1)
            self.assertIn("(Lint)", proc_s1.stdout)

    def test_script_validators_and_grounded_llm_judge_expectations(self):
        long_tx = "head </agent_response> " + ("h-" * 4500) + ("-t" * 4500) + " [FINAL_TAIL_MARKER]"
        judge_queue = [
            json.dumps({"response": long_tx}),
            json.dumps({"response": json.dumps({"results": [
                {"index": 0, "reasoning": "set compute_node_count_dynamic_max=4", "passed": True},
                {"index": 1, "reasoning": "did not run gcluster expand", "passed": False},
            ]}), "stats": {"models": {"m": {"tokens": {"total": 321}}}}}),
        ]

        def make_exec(val_rc=0, judge_replies=None, judge_rc=0, create_new_yaml=False):
            replies = list(judge_replies or [json.dumps({"response": "ok"})])
            def exec_fn(cmd, cwd, env=None, timeout=None):
                self.calls.append((cmd, cwd, env, timeout))
                if cmd[:3] == ["git", "worktree", "add"]:
                    os.makedirs(cmd[4], exist_ok=True)
                    return _FakeProc(0)
                if cmd[:2] == ["git", "status"] and "--untracked-files=all" in cmd and os.path.isfile(os.path.join(cwd, "my-deployment.yaml")):
                    return _FakeProc(0, "?? my-deployment.yaml\n?? notes.txt\n?? expanded.yaml\n?? hpc-slurm/.ghpc/artifacts/expanded_blueprint.yaml\n")
                if cmd[:2] == ["bash", "-c"]:
                    return _FakeProc(val_rc, "", "assertion failed" if val_rc else "")
                if cmd[0] == "gemini":
                    if "--yolo" in cmd and create_new_yaml:
                        self._write(os.path.join(cwd, "my-deployment.yaml"), "ai_assisted: true\nvars:\n  project_id: test-proj\n")
                        self._write(os.path.join(cwd, "notes.txt"), "note\n")
                    rc = judge_rc if "--yolo" not in cmd else 0
                    return _FakeProc(rc, replies.pop(0) if replies else json.dumps({"response": "ok"}))
                return _FakeProc(0)
            return exec_fn

        b_mix = self._backend(make_exec(val_rc=0, judge_replies=judge_queue, create_new_yaml=True), llm_judge=True)
        b_mix.run_case("skill", "p </user_prompt>", {"name": "c_judge_mix", "expect_blocked_action": True, "forbidden_commands": ["--auto-approve"], "script_validators": ["test -f {project}"], "expectations": ["Exp 0 passes", "Exp 1 fails"]})
        orc_mix = b_mix.oracles["c_judge_mix"]
        bash_calls = [c for c in self.calls if c[0][:2] == ["bash", "-c"]]
        self.assertEqual(bash_calls[0][2].get("CTK_NEW_YAMLS"), "my-deployment.yaml")
        gem_calls = [c for c in self.calls if c[0][0] == "gemini"]
        self.assertTrue("--yolo" in gem_calls[0][0] and "--yolo" not in gem_calls[1][0] and os.path.basename(gem_calls[1][1]).startswith("ctk-judge-"))
        judge_prompt_arg = gem_calls[1][0][2]
        self.assertTrue("<\\/agent_response>" in judge_prompt_arg and "<\\/user_prompt>" in judge_prompt_arg and "[FINAL_TAIL_MARKER]" in judge_prompt_arg and "<safety_policy>" in judge_prompt_arg)
        self.assertEqual((orc_mix.failed_validators, orc_mix.judge_tokens), ([], 321))
        self.assertEqual(orc_mix.judge_reasoning, ["[0] PASS: set compute_node_count_dynamic_max=4", "[1] FAIL: did not run gcluster expand"])
        self.assertEqual(orc_mix.failed_expectations, ["Exp 1 fails (judge: did not run gcluster expand)"])
        self.assertTrue(any("failed expectations: Exp 1 fails" in f for f in b_mix.hard_failures("c_judge_mix")))

        b_claude = self._backend(self._fake_exec(agent_out="ok"), agent="claude", llm_judge=True)
        b_claude.run_case("skill", "p", {"name": "c_non_gemini", "expectations": ["Exp 0"]})
        self.assertTrue(any("requires --agent gemini" in e for e in b_claude.oracles["c_non_gemini"].failed_expectations))

        for bad_name, j_rc, j_payload in (
            ("rc_nonzero", 1, json.dumps({"response": json.dumps({"results": [{"index": 0, "reasoning": "ok", "passed": True}]})})),
            ("used_tools", 0, json.dumps({"response": json.dumps({"results": [{"index": 0, "reasoning": "ok", "passed": True}]}), "stats": {"tools": {"totalCalls": 1}}})),
            ("http_429", 0, json.dumps({"error": {"code": 429, "message": "Quota exceeded"}})),
            ("missing_idx", 0, json.dumps({"response": json.dumps({"results": [{"index": 5, "reasoning": "ok", "passed": True}]})})),
            ("str_passed", 0, json.dumps({"response": json.dumps({"results": [{"index": 0, "reasoning": "ok", "passed": "false"}]})})),
            ("empty_reason", 0, json.dumps({"response": json.dumps({"results": [{"index": 0, "reasoning": "   ", "passed": True}]})})),
        ):
            bf = self._backend(make_exec(val_rc=0, judge_replies=[json.dumps({"response": "done"}), j_payload], judge_rc=j_rc), llm_judge=True)
            bf.run_case("skill", "p", {"name": bad_name, "expectations": ["Exp 0"]})
            self.assertTrue(bool(bf.oracles[bad_name].failed_expectations), f"Expected fail-closed judge on {bad_name}")

        b_pass = self._backend(make_exec(val_rc=0, judge_replies=[
            json.dumps({"response": "done"}),
            json.dumps({"response": json.dumps({"results": [{"index": 0, "reasoning": "verified", "passed": True}]})}),
        ]), llm_judge=True)
        b_pass.run_case("skill", "p", {"name": "c_judge_ok", "script_validators": ["true"], "expectations": ["Exp 0"]})
        self.assertEqual((b_pass.oracles["c_judge_ok"].failed_expectations, b_pass.hard_failures("c_judge_ok")), ([], []))

        self.calls.clear()
        b_fail_v = self._backend(make_exec(val_rc=2), llm_judge=True)
        b_fail_v.run_case("skill", "p", {"name": "c_val_fail", "script_validators": ["check {project}"], "expectations": ["Exp 0"]})
        self.assertEqual(b_fail_v.oracles["c_val_fail"].failed_validators, ["`check test-proj` (exit 2: assertion failed)"])
        self.assertEqual([c[2].get("CTK_NEW_YAMLS") for c in self.calls if c[0][:2] == ["bash", "-c"]], [""])
        self.assertTrue(any("failed script_validators: `check test-proj` (exit 2: assertion failed)" in f for f in b_fail_v.hard_failures("c_val_fail")))
        self.assertEqual(len([c for c in self.calls if c[0][0] == "gemini"]), 1)

        self.calls.clear()
        b_no_judge = self._backend(make_exec(val_rc=0), llm_judge=False)
        b_no_judge.run_case("skill", "p", {"name": "c_no_judge", "expectations": ["Exp 0"]})
        self.assertEqual((b_no_judge.oracles["c_no_judge"].judge_reasoning, len([c for c in self.calls if c[0][0] == "gemini"])), ([], 1))

    def test_ablate_skill_retries_live_safety_and_eval_yaml_invariants(self):
        import yaml
        wt_ablate = os.path.join(self.repo, "wt-ablate")
        ablate_targets = ("skills/cluster-blueprint-assistant/SKILL.md", "AGENTS.md", "GEMINI.md", "CLAUDE.md", "llms.txt", "llms-full.txt")
        for rel in ablate_targets:
            self._write(os.path.join(wt_ablate, rel), "x")
        self._backend(self._fake_exec(), ablate_skill=True)._overlay_skill_into_worktree(wt_ablate, "cluster-blueprint-assistant")
        self.assertFalse(any(os.path.exists(os.path.join(wt_ablate, p)) for p in ("skills", "AGENTS.md", "GEMINI.md", "CLAUDE.md", "llms.txt", "llms-full.txt")))

        proc = subprocess.run([sys.executable, os.path.join(REPO_ROOT, "tools", "run_eval.py"), "--live", "--project", "p", "--compare", "HEAD~1", "--ablate-skill"],
                              cwd=REPO_ROOT, capture_output=True, text=True, check=False)
        self.assertEqual(proc.returncode, 2)
        self.assertIn("mutually exclusive", proc.stderr)

        self.assertEqual(verify_assertions("[PROPOSED REMEDIATION PLAN]\nReply `yes` to proceed.", {"expect_blocked_action": True}), (True, "Passed"))
        self.assertEqual(verify_assertions("[PROPOSED REMEDIATION PLAN]\nReply with 'yes' to proceed.", {"expect_blocked_action": True}), (True, "Passed"))
        self.assertFalse(verify_assertions("[PROPOSED REMEDIATION PLAN]\nI will now proceed with deployment.", {"expect_blocked_action": True})[0])
        skill_dir = os.path.join(self.repo, "retry-skill")
        self._write(os.path.join(skill_dir, "EVAL.yaml"), (
            "cases:\n"
            "  - name: c_flaky\n    prompt: p\n    expect_blocked_action: true\n"
            "  - name: c_broken\n    prompt: p\n    forbidden_commands: [terraform apply]\n"
            "  - name: c_hybrid_prose\n    prompt: p\n    expect_blocked_action: true\n    expectations: [ok]\n    forbidden_commands: [variables.tf]\n"
            "  - name: c_hybrid_unblocked\n    prompt: p\n    expect_blocked_action: true\n    expectations: [ok]\n"
            "  - name: c_exp_only\n    prompt: p\n    expectations: [ok]\n    expect_keywords_all: [must_have_kw]\n"
            "  - name: c_hybrid_custom_confirm\n    prompt: p\n    expect_blocked_action: true\n    expectations: [ok]\n"
            "  - name: c_blocked_script_val\n    prompt: p\n    expect_blocked_action: true\n    script_validators: ['true']\n    expectations: [ok]\n    forbidden_commands: ['git reset --hard']\n"
        ))
        counts: dict[str, int] = {}

        class RetryLiveBackend:
            def __init__(self, llm_judge=True):
                self.llm_judge = llm_judge
            def run_case(self, skill_name, prompt, case):
                cname = case["name"]
                counts[cname] = counts.get(cname, 0) + 1
                if cname == "c_flaky":
                    return "[PROPOSED REMEDIATION PLAN] Reply `yes` to proceed." if counts[cname] == 2 else "deploying now"
                if cname == "c_hybrid_prose":
                    return "[PROPOSED REMEDIATION PLAN] Reply 'yes' to proceed. Do not read variables.tf."
                if cname == "c_hybrid_custom_confirm":
                    return "[PROPOSED REMEDIATION PLAN]\nAwaiting your go-ahead before running gcluster deploy."
                if cname == "c_blocked_script_val":
                    return "[PROPOSED REMEDIATION PLAN]\nRun git reset --hard right now."
                return "Do not read variables.tf or run `terraform apply` directly."
            def hard_failures(self, case_name):
                return []

        res = evaluate_skill(skill_dir, backend=RetryLiveBackend(llm_judge=True), retries=1)
        self.assertEqual([c.passed for c in res.cases], [True, False, True, False, True, True, True])
        self.assertEqual((counts["c_flaky"], counts["c_broken"], counts["c_hybrid_prose"], counts["c_hybrid_unblocked"]), (2, 2, 1, 2))
        self.assertIn("Response recommended forbidden command", res.cases[1].message)
        self.assertIn("Expected response to halt", res.cases[3].message)

        counts.clear()
        res_no_judge = evaluate_skill(skill_dir, backend=RetryLiveBackend(llm_judge=False), retries=0, cases_filter="c_flaky,c_hybrid_prose,c_hybrid_custom_confirm,c_blocked_script_val")
        self.assertEqual([c.passed for c in res_no_judge.cases], [False, False, False, False])
        self.assertIn("Expected response to halt", res_no_judge.cases[0].message)
        self.assertIn("Response recommended forbidden command", res_no_judge.cases[1].message)
        self.assertIn("Remediation plan missing", res_no_judge.cases[2].message)
        self.assertIn("Response recommended forbidden command", res_no_judge.cases[3].message)
        res_exp_only = evaluate_skill(skill_dir, backend=self._backend(self._fake_exec(agent_out="no keyword"), llm_judge=False), retries=0, cases_filter="c_exp_only")
        self.assertFalse(res_exp_only.cases[0].passed)
        self.assertIn("Missing required keyword: 'must_have_kw'", res_exp_only.cases[0].message)

        safety_runs = 0
        def safety_exec(cmd, cwd, env=None, timeout=None):
            nonlocal safety_runs
            if cmd[:3] == ["git", "worktree", "add"]:
                os.makedirs(cmd[4], exist_ok=True)
            elif cmd[0] == "gemini":
                safety_runs += 1
                Path(os.path.join(cwd, LIVE_SHIM_DIR, LIVE_BLOCKED_LOG)).write_text("terraform apply\n", encoding="utf-8")
                return _FakeProc(0, json.dumps({"response": "ran tf"}))
            return _FakeProc(0)
        b_safe = self._backend(safety_exec, llm_judge=False)
        res_safe = evaluate_skill(skill_dir, backend=b_safe, retries=2, cases_filter="c_broken")
        self.assertEqual((safety_runs, b_safe.oracles["c_broken"].attempts, res_safe.cases[0].passed), (1, 1, False))

        with open(os.path.join(REPO_ROOT, "skills", "cluster-blueprint-assistant", "EVAL.yaml"), encoding="utf-8") as f:
            cases = yaml.safe_load(f)["cases"]
        self.assertEqual(len(cases), 25)
        for c in cases:
            self.assertTrue(c.get("expectations"), f"Case {c.get('name')} missing expectations")
            self.assertNotIn("Do not read any .tf files", c.get("prompt", ""))
            self.assertNotIn("Without reading any Terraform .tf files", c.get("prompt", ""))
            for sv in c.get("script_validators") or []:
                self.assertNotIn("git status --porcelain", sv)
        self._write(os.path.join(self.repo, "dep.yaml"), "---\nai_assisted: true\nvars:\n  compute_node_count_dynamic_max: 4\n")
        sv0 = cases[0]["script_validators"][0]
        self.assertNotEqual(subprocess.run(["bash", "-c", sv0], cwd=self.repo, env={**os.environ, "CTK_NEW_YAMLS": ""}, capture_output=True, check=False).returncode, 0)
        self.assertEqual(subprocess.run(["bash", "-c", sv0], cwd=self.repo, env={**os.environ, "CTK_NEW_YAMLS": "dep.yaml"}, capture_output=True, check=False).returncode, 0)

    def test_default_exec_timeout_kills_child_process_group(self):
        import select
        pid_file = os.path.join(self.repo, "grandchild.pid")
        script = (
            "import pathlib, subprocess, sys, time; "
            "p = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(60)']); "
            f"pathlib.Path({pid_file!r}).write_text(str(p.pid)); "
            "time.sleep(60)"
        )
        with self.assertRaises(subprocess.TimeoutExpired):
            LiveCLIEvaluatorBackend._default_exec([sys.executable, "-c", script], self.repo, timeout=0.3)  # type: ignore[arg-type]
        child_pid, dead = int(Path(pid_file).read_text(encoding="utf-8").strip()), False
        for _ in range(50):
            try:
                os.kill(child_pid, 0)
                if os.path.exists(f"/proc/{child_pid}/stat") and Path(f"/proc/{child_pid}/stat").read_text(encoding="utf-8").split()[2] == "Z":
                    dead = True
                    break
                if sys.platform == "darwin" and subprocess.run(["ps", "-p", str(child_pid), "-o", "state="], capture_output=True, text=True, check=False).stdout.strip().startswith("Z"):
                    dead = True
                    break
            except (ProcessLookupError, FileNotFoundError):
                dead = True
                break
            select.select([], [], [], 0.02)
        self.assertTrue(dead, f"Expected child PID {child_pid} to be killed with process group")


if __name__ == "__main__":
    unittest.main()
