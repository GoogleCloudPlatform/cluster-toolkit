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

"""Unit tests for .github/scripts/breaking_check_comment.py and breaking_check_gate.py."""

import importlib.util
import json
import os
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = REPO_ROOT / ".github" / "scripts"


def _load(name):
    spec = importlib.util.spec_from_file_location(name, SCRIPTS / f"{name}.py")
    mod = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(mod)
    return mod


comment = _load("breaking_check_comment")
gate = _load("breaking_check_gate")

BLOCK = """Some preamble the model wrote.
<!-- release-breaking-check -->
VERDICT: BREAKING
CONFIDENCE: high
SEVERITY: high
WHY: examples/a3ultra-slurm-blueprint.yaml swaps homefs source filestore -> managed-lustre;
the Filestore is destroyed on redeploy.
SIGNALS: example_module_id_or_source_changed, example_var_or_setting_removed
UPGRADE_STEPS:
- Back up /home before redeploying.
- Remove filestore_ip_range from deployment.yaml.
REDEPLOY_IMPACT: Destroys the /home Filestore and creates Managed Lustre in its place.
"""

FILLED_BODY = """Intro

#### Upgrade steps

- Back up /home.

#### Redeploy impact

Destroys the Filestore.
"""
EMPTY_BODY = """#### Upgrade steps

<!-- Required if breaking. -->

#### Redeploy impact

<!-- One line. -->
"""


class ParseTests(unittest.TestCase):
    def test_parses_full_block(self):
        v = comment.parse_verdict(BLOCK)
        self.assertEqual(v["VERDICT"], "BREAKING")
        self.assertEqual(v["SEVERITY"], "high")
        self.assertIn("Filestore is destroyed", v["WHY"])
        self.assertEqual(len(v["UPGRADE_STEPS"]), 2)
        self.assertTrue(v["REDEPLOY_IMPACT"].startswith("Destroys"))

    def test_strips_code_fences_and_handles_none(self):
        v = comment.parse_verdict("```text\nVERDICT: NOT_BREAKING\nSEVERITY: none\nWHY: additive\nSIGNALS: none\nUPGRADE_STEPS:\n- none\nREDEPLOY_IMPACT: none\n```")
        self.assertEqual(v["VERDICT"], "NOT_BREAKING")
        self.assertEqual(v["UPGRADE_STEPS"], [])
        self.assertEqual(v["REDEPLOY_IMPACT"], "")

    def test_invalid_or_missing_verdict_returns_none(self):
        self.assertIsNone(comment.parse_verdict(""))
        self.assertIsNone(comment.parse_verdict("VERDICT: MAYBE\n"))
        self.assertIsNone(comment.parse_verdict("no block here"))


class RenderTests(unittest.TestCase):
    scan = {"signals": [{"id": "example_deleted"}], "signal_count": 1, "severity_hint": "medium", "touched_examples": ["examples/x.yaml"]}

    def test_breaking_without_label_suggests_label_and_sections(self):
        body = comment.render_comment(comment.parse_verdict(BLOCK), self.scan, {"labels": ["release-improvements"]}, "advisory")
        self.assertIn(comment.MARKER, body)
        self.assertIn("add the label `release-breaking-changes`", body)
        self.assertIn("#### Upgrade steps", body)
        self.assertIn("Back up /home", body)

    def test_fork_comment_has_no_verdict(self):
        body = comment.render_comment(None, self.scan, {"labels": []}, "advisory", fork=True)
        self.assertIn("comes from a fork", body)
        self.assertNotIn("Verdict:", body)

    def test_not_breaking_with_label_mentions_it(self):
        v = comment.parse_verdict("VERDICT: NOT_BREAKING\nWHY: additive\n")
        body = comment.render_comment(v, self.scan, {"labels": ["release-breaking-changes"]}, "enforce")
        self.assertIn("found no breaking effect", body)


class GateTests(unittest.TestCase):
    maint = {"alice", "bob"}

    def ev(self, label, actor, event="labeled"):
        return {"event": event, "label": {"name": label}, "actor": {"login": actor}}

    def test_breaking_without_label_fails(self):
        f, _ = gate.evaluate({"VERDICT": "BREAKING"}, ["release-improvements"], FILLED_BODY, {}, [])
        self.assertTrue(any("lacks the" in x for x in f))

    def test_breaking_with_label_and_sections_passes(self):
        f, _ = gate.evaluate({"VERDICT": "BREAKING"}, ["release-breaking-changes", "release-improvements"], FILLED_BODY, {}, [])
        self.assertEqual(f, [])

    def test_label_with_empty_sections_fails_even_if_not_breaking(self):
        f, _ = gate.evaluate({"VERDICT": "NOT_BREAKING"}, ["release-breaking-changes"], EMPTY_BODY, {}, [])
        self.assertEqual(len(f), 2)

    def test_needs_plan_check_fails_without_override(self):
        f, _ = gate.evaluate({"VERDICT": "NEEDS_PLAN_CHECK"}, [], FILLED_BODY, {}, [])
        self.assertTrue(any("NEEDS_PLAN_CHECK" in x for x in f))

    def test_no_verdict_and_no_label_passes(self):
        f, _ = gate.evaluate(None, ["release-bugfix"], "", {}, [])
        self.assertEqual(f, [])

    def test_override_by_maintainer_not_author_is_trusted(self):
        ov = gate.valid_overrides([self.ev("breaking-check/not-breaking", "alice")], self.maint, author="carol")
        self.assertEqual(ov, {"breaking-check/not-breaking": "alice"})
        f, notes = gate.evaluate({"VERDICT": "BREAKING"}, [], FILLED_BODY, ov, [])
        self.assertEqual(f, [])
        self.assertTrue(any("accepted" in n for n in notes))

    def test_override_by_author_is_rejected(self):
        ov = gate.valid_overrides([self.ev("breaking-check/not-breaking", "alice")], self.maint, author="alice")
        self.assertEqual(ov, {})
        f, notes = gate.evaluate({"VERDICT": "BREAKING"}, ["breaking-check/not-breaking"], FILLED_BODY, ov, ["breaking-check/not-breaking"])
        self.assertTrue(f)
        self.assertTrue(any("ignored" in n for n in notes))

    def test_override_by_non_maintainer_is_rejected(self):
        ov = gate.valid_overrides([self.ev("breaking-check/not-breaking", "mallory")], self.maint, author="carol")
        self.assertEqual(ov, {})

    def test_unlabel_then_relabel_tracks_latest_actor(self):
        events = [self.ev("breaking-check/not-breaking", "mallory"),
                  self.ev("breaking-check/not-breaking", "mallory", "unlabeled"),
                  self.ev("breaking-check/not-breaking", "bob")]
        self.assertEqual(gate.valid_overrides(events, self.maint, "carol"), {"breaking-check/not-breaking": "bob"})

    def test_plan_verified_override_turns_needs_plan_check_into_breaking(self):
        ov = {"breaking-check/plan-verified": "bob"}
        f, _ = gate.evaluate({"VERDICT": "NEEDS_PLAN_CHECK"}, ["release-breaking-changes"], FILLED_BODY, ov, [])
        self.assertEqual(f, [])
        f, _ = gate.evaluate({"VERDICT": "NEEDS_PLAN_CHECK"}, [], FILLED_BODY, ov, [])
        self.assertTrue(any("lacks the" in x for x in f))

    def test_section_text_ignores_comments_and_whitespace(self):
        self.assertEqual(gate.section_text(EMPTY_BODY, "Upgrade steps"), "")
        self.assertEqual(gate.section_text(FILLED_BODY, "Redeploy impact"), "DestroystheFilestore.")

    def test_cli_modes(self):
        with tempfile.TemporaryDirectory() as d:
            pr = {"number": 1, "labels": [{"name": "release-improvements"}], "author": {"login": "carol"}, "body": ""}
            prf = os.path.join(d, "pr.json"); json.dump(pr, open(prf, "w"))
            vf = os.path.join(d, "v.json"); json.dump({"VERDICT": "BREAKING"}, open(vf, "w"))
            mf = os.path.join(d, "m.json"); json.dump([{"login": "alice"}], open(mf, "w"))
            ef = os.path.join(d, "e.json"); json.dump([], open(ef, "w"))
            base = ["--pr-file", prf, "--verdict-json", vf, "--maintainers", mf, "--events-file", ef]
            self.assertEqual(gate.main(base + ["--mode", "advisory"]), 0)
            self.assertEqual(gate.main(base + ["--mode", "enforce"]), 1)


if __name__ == "__main__":
    unittest.main(verbosity=2)
