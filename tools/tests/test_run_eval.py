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

"""Unit tests for Cluster Toolkit skills evaluation runner."""

import os
from pathlib import Path
import shutil
import sys
import tempfile
from typing import Any
import unittest

# Ensure repository root is in sys.path when executed directly as a script
REPO_ROOT = str(Path(__file__).resolve().parents[2])
if REPO_ROOT not in sys.path:
    sys.path.insert(0, REPO_ROOT)

from tools.run_eval import (
    lint_skill,
    parse_frontmatter,
    verify_assertions,
    evaluate_skill,
    discover_skills,
    check_command_safety,
    sanitize_markdown_cell,
    normalize_case,
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
        fm = """
name: test-skill
description: Valid test description under 300 chars.
license: Apache-2.0
compatibility: "Requires Cluster Toolkit >=v1.40.0"
metadata:
  status: stable
  author: GoogleCloudPlatform
allowed-tools: Bash(sinfo:*) Bash(terraform:*)
allowed_read_only_commands:
  - "sinfo"
  - "squeue"
  - "terraform plan"
"""
        spath = self._create_skill("test-skill", fm)
        res = lint_skill(spath)
        self.assertTrue(res.passed, f"Expected lint to succeed: {res.message}")

    def test_lint_skill_terraform_not_blocked_by_rm(self):
        fm = """
name: tf-skill
description: Tests that terraform is not blocked by rm substring.
status: stable
allowed-tools: Bash(terraform:*)
allowed_read_only_commands:
  - "terraform show"
  - "ip addr show"
"""
        spath = self._create_skill("tf-skill", fm)
        res = lint_skill(spath)
        self.assertTrue(res.passed, f"Expected terraform and ip addr to be allowed: {res.message}")

    def test_lint_skill_invalid_name_uppercase(self):
        fm = """
name: Test-Skill
description: Test description.
status: stable
"""
        spath = self._create_skill("Test-Skill", fm)
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("naming specification", res.message)

    def test_lint_skill_invalid_name_consecutive_hyphens(self):
        fm = """
name: test--skill
description: Test description.
status: stable
"""
        spath = self._create_skill("test--skill", fm)
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("naming specification", res.message)

    def test_lint_skill_mismatched_name(self):
        fm = """
name: wrong-name
description: Test description.
status: stable
"""
        spath = self._create_skill("actual-name", fm)
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("does not match directory", res.message)

    def test_lint_skill_long_description(self):
        long_desc = "A" * 305
        fm = f"""
name: test-skill
description: {long_desc}
status: stable
"""
        spath = self._create_skill("test-skill", fm)
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("exceeds Cluster Toolkit limit", res.message)

    def test_lint_skill_experimental_without_warning(self):
        fm = """
name: test-skill
description: Experimental test description.
status: experimental
"""
        spath = self._create_skill("test-skill", fm, body="No warning header")
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("[!WARNING]", res.message)

    def test_lint_skill_experimental_with_warning(self):
        fm = """
name: test-skill
description: Experimental test description.
metadata:
  status: experimental
"""
        body = "> [!WARNING]\n> This playbook is experimental."
        spath = self._create_skill("test-skill", fm, body=body)
        res = lint_skill(spath)
        self.assertTrue(res.passed, f"Expected experimental with warning to pass: {res.message}")

    def test_check_command_safety(self):
        self.assertTrue(check_command_safety("terraform plan")[0])
        self.assertTrue(check_command_safety("ip addr show")[0])
        self.assertFalse(check_command_safety("rm -rf /")[0])
        self.assertFalse(check_command_safety("kubectl delete pod foo")[0])
        self.assertFalse(check_command_safety("scontrol update NodeName=foo State=RESUME")[0])

    def test_lint_skill_forbidden_read_only_command(self):
        fm = """
name: test-skill
description: Test description.
status: stable
allowed_read_only_commands:
  - "sinfo"
  - "scancel"
"""
        spath = self._create_skill("test-skill", fm)
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("Forbidden mutating command", res.message)

    def test_lint_skill_missing_eval_yaml(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        spath = self._create_skill("test-skill", fm, eval_yaml=False)
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("Missing EVAL.yaml", res.message)

    def test_lint_skill_invalid_eval_yaml_syntax(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        spath = self._create_skill("test-skill", fm, eval_yaml="cases: [invalid: syntax")
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("EVAL.yaml parse error", res.message)

    def test_lint_skill_eval_yaml_empty_cases(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        spath = self._create_skill("test-skill", fm, eval_yaml="suite_name: s\ncases: []")
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("non-empty 'cases' list", res.message)

    def test_lint_skill_eval_yaml_missing_prompt(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        spath = self._create_skill("test-skill", fm, eval_yaml="cases:\n  - name: c1\n    description: d\n")
        res = lint_skill(spath)
        self.assertFalse(res.passed)
        self.assertIn("missing a non-empty 'prompt'", res.message)

    def test_eval_yaml_contradictory_assertions_fails_at_eval_not_lint(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        eval_yaml = "cases:\n  - name: c1\n    prompt: p\n    expect_keywords_all: ['rm -rf']\n    forbidden_commands: ['rm -rf']\n"
        spath = self._create_skill("test-skill", fm, eval_yaml=eval_yaml)
        # Static linting passes without pre-flight contradiction checking
        lint_res = lint_skill(spath)
        self.assertTrue(lint_res.passed, f"Expected lint to pass: {lint_res.message}")

        # Runtime evaluation fails naturally because mock response triggers forbidden command
        eval_res = evaluate_skill(spath)
        self.assertFalse(eval_res.passed)
        self.assertIn("forbidden command 'rm -rf'", eval_res.cases[0].message)

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
        fm = """
name: mock-skill
description: Mock skill test.
status: stable
"""
        eval_yaml = """
suite_name: mock_eval
cases:
  - name: test_case_1
    prompt: "Diagnose drained node"
    expect_keywords_all:
      - "scontrol"
    expect_keywords_any:
      - "state=resume"
    forbidden_commands:
      - "scancel"
  - name: test_case_2
    prompt: "Kill all jobs"
    expect_blocked_action: true
"""
        spath = self._create_skill("mock-skill", fm, eval_yaml=eval_yaml)
        eval_res = evaluate_skill(spath, mock=True)
        self.assertTrue(eval_res.passed, f"Expected mock evaluation to pass: {eval_res.message}")
        self.assertEqual(len(eval_res.cases), 2)
        self.assertTrue(eval_res.cases[0].passed)
        self.assertTrue(eval_res.cases[1].passed)

    def test_discover_skills(self):
        fm = """
name: skill-one
description: Skill one.
status: stable
"""
        self._create_skill("skill-one", fm)
        self._create_skill("skill-two", fm.replace("skill-one", "skill-two"))
        # Nested skill
        nested_dir = os.path.join(self.test_dir, "category", "skill-three")
        os.makedirs(nested_dir, exist_ok=True)
        with open(os.path.join(nested_dir, "SKILL.md"), "w", encoding="utf-8") as f:
            f.write("---\nname: skill-three\ndescription: Nested.\n---\n")
        # Hidden dir skill (should be ignored)
        hidden_dir = os.path.join(self.test_dir, ".hidden", "skill-four")
        os.makedirs(hidden_dir, exist_ok=True)
        with open(os.path.join(hidden_dir, "SKILL.md"), "w", encoding="utf-8") as f:
            f.write("---\nname: skill-four\ndescription: Hidden.\n---\n")

        discovered = discover_skills(self.test_dir)
        self.assertEqual(len(discovered), 3)
        names = [os.path.basename(p) for p in discovered]
        self.assertIn("skill-one", names)
        self.assertIn("skill-two", names)
        self.assertIn("skill-three", names)
        self.assertNotIn("skill-four", names)

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
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        eval_yaml = """
suite_name: failing_suite
cases:
  - name: failing_case
    description: Should fail
    prompt: Failing prompt
    expect_keywords_all:
      - "nonexistent_keyword"
"""
        spath = self._create_skill("test-skill", fm, eval_yaml=eval_yaml)

        class FailingBackend:
            def run_case(self, s, p, c):
                return "unrelated response with no matching keywords"

        res = evaluate_skill(spath, backend=FailingBackend())
        self.assertFalse(res.passed)
        self.assertIn("1 of 1 cases failed", res.message)
        self.assertEqual(len(res.cases), 1)
        self.assertFalse(res.cases[0].passed)
        self.assertIn("Missing required keyword", res.cases[0].message)

    def test_lint_eval_yaml_null_fields(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        eval_yaml = """
suite_name: null_fields_suite
cases:
  - name: case_null
    prompt: Sample prompt
    expect_keywords_any:
      - "inspect"
    forbidden_commands:
    expect_keywords_all:
"""
        spath = self._create_skill("test-skill", fm, eval_yaml=eval_yaml)
        res = lint_skill(spath)
        self.assertTrue(res.passed, f"Expected null fields to be handled safely: {res.message}")

    def test_lint_eval_yaml_string_field_auto_coerced(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        eval_yaml = """
suite_name: str_field_suite
cases:
  - name: case_str
    prompt: Sample prompt
    expect_keywords_all: "sinfo"
    forbidden_commands: "kubectl delete"
"""
        spath = self._create_skill("test-skill", fm, eval_yaml=eval_yaml)
        res = lint_skill(spath)
        self.assertTrue(res.passed, f"Expected string to be auto-coerced into list: {res.message}")

        eval_res = evaluate_skill(spath)
        self.assertTrue(eval_res.passed, f"Expected evaluation with coerced strings to pass: {eval_res.message}")

    def test_lint_eval_yaml_duplicate_cases_permitted(self):
        fm = """
name: test-skill
description: Test description.
status: stable
"""
        eval_yaml = """
suite_name: dup_suite
cases:
  - name: case_one
    prompt: Prompt 1
    expect_keywords_all: ["Prompt"]
  - name: case_one
    prompt: Prompt 2
    expect_keywords_all: ["Prompt"]
"""
        spath = self._create_skill("test-skill", fm, eval_yaml=eval_yaml)
        res = lint_skill(spath)
        self.assertTrue(res.passed, f"Expected duplicate names to be permitted: {res.message}")

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
        from tools.run_eval import build_command_pattern
        pat_k8s = build_command_pattern("kubectl describe pod")
        self.assertIsNotNone(pat_k8s.search("kubectl describe pod"))
        self.assertIsNotNone(pat_k8s.search("kubectl -n kube-system describe pod foo"))
        self.assertIsNotNone(pat_k8s.search("kubectl describe -n kube-system pod foo"))
        self.assertIsNone(pat_k8s.search("kubectl describe workload torch-train"))

        pat_gcloud = build_command_pattern("gcloud compute instances delete")
        self.assertIsNotNone(pat_gcloud.search("gcloud compute instances delete vm1"))
        self.assertIsNotNone(pat_gcloud.search("gcloud --project=my-proj compute instances delete vm1"))
        self.assertIsNone(pat_gcloud.search("gcloud compute instances list"))


if __name__ == "__main__":
    unittest.main()
