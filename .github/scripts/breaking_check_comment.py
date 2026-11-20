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

"""Shared helpers for the release-breaking check workflow.

Parses the output block defined in skills/release-breaking-check/SKILL.md and
renders / upserts the sticky PR comment. Standard library only.
"""

import json
import os
import re
import subprocess
import sys
from typing import Dict, List, Optional

MARKER = "<!-- release-breaking-check -->"
VERDICTS = ("BREAKING", "NOT_BREAKING", "NEEDS_PLAN_CHECK")
FIELDS = ("VERDICT", "CONFIDENCE", "SEVERITY", "WHY", "SIGNALS", "REDEPLOY_IMPACT")


def parse_verdict(text: str) -> Optional[Dict]:
    """Parse the skill's output block. Returns None if no VERDICT line is found."""
    if not text or "VERDICT:" not in text:
        return None
    # Keep only the block from the marker (or first VERDICT) onward.
    start = text.find(MARKER)
    if start < 0:
        start = text.find("VERDICT:")
    body = text[start:]
    body = re.sub(r"^```.*?$", "", body, flags=re.MULTILINE)  # strip code fences if the model added them

    out: Dict = {k: "" for k in FIELDS}
    out["UPGRADE_STEPS"] = []
    cur = None
    for raw in body.splitlines():
        line = raw.rstrip()
        m = re.match(r"^([A-Z_]+):\s*(.*)$", line)
        if m and m.group(1) in FIELDS + ("UPGRADE_STEPS",):
            key, val = m.group(1), m.group(2).strip()
            cur = key
            if key == "UPGRADE_STEPS":
                if val and val != "-":
                    out["UPGRADE_STEPS"].append(val.lstrip("- ").strip())
            else:
                out[key] = val
            continue
        if cur == "UPGRADE_STEPS" and line.strip().startswith(("-", "*")):
            out["UPGRADE_STEPS"].append(line.strip().lstrip("-* ").strip())
        elif cur == "WHY" and line.strip() and not re.match(r"^[A-Z_]+:", line):
            out["WHY"] += " " + line.strip()

    out["VERDICT"] = out["VERDICT"].upper().split()[0] if out["VERDICT"] else ""
    if out["VERDICT"] not in VERDICTS:
        return None
    out["CONFIDENCE"] = (out["CONFIDENCE"] or "medium").lower().split()[0]
    out["SEVERITY"] = (out["SEVERITY"] or "none").lower().split()[0]
    out["UPGRADE_STEPS"] = [s for s in out["UPGRADE_STEPS"] if s and s.lower() != "none"]
    if out["REDEPLOY_IMPACT"].strip().lower() == "none":
        out["REDEPLOY_IMPACT"] = ""
    return out


def render_comment(verdict: Optional[Dict], scan: Dict, pr: Dict, mode: str, fork: bool = False) -> str:
    """Render the sticky comment body."""
    icon = {"BREAKING": "🚨", "NOT_BREAKING": "✅", "NEEDS_PLAN_CHECK": "🔎"}
    lines = [MARKER, "### Release-breaking check"]
    sig_ids = sorted({s["id"] for s in scan.get("signals", [])})
    sig_txt = ", ".join(sig_ids) if sig_ids else "none"

    if fork or verdict is None:
        lines.append(f"**Automated diff scan:** {scan.get('signal_count', 0)} signal(s) — {sig_txt} "
                     f"(severity hint: `{scan.get('severity_hint', 'none')}`).")
        if scan.get("touched_examples"):
            lines.append("Touched examples: " + ", ".join(f"`{p}`" for p in scan["touched_examples"]))
        if fork:
            lines.append("")
            lines.append("This PR comes from a fork, so the model review did not run. A maintainer can run it from "
                         "**Actions → Release-breaking change check → Run workflow** with this PR number.")
        elif scan.get("signal_count", 0) == 0:
            lines.append("")
            lines.append("No mechanical signals; model review skipped.")
        else:
            lines.append("")
            lines.append("Model review did not produce a parsable verdict; please apply "
                         "`skills/release-breaking-check/SKILL.md` manually.")
    else:
        v = verdict["VERDICT"]
        lines.append(f"**Verdict:** {icon.get(v, '')} `{v}` · confidence `{verdict['CONFIDENCE']}` · severity `{verdict['SEVERITY']}`")
        lines.append("")
        lines.append(f"**Why:** {verdict['WHY'].strip()}")
        lines.append(f"**Signals:** {verdict['SIGNALS'] or sig_txt}")
        labels = [l for l in pr.get("labels", [])]
        has_label = "release-breaking-changes" in labels
        if v == "BREAKING":
            lines.append("")
            if has_label:
                lines.append("Label `release-breaking-changes` is present. Make sure the description's **Upgrade steps** and **Redeploy impact** are filled in.")
            else:
                lines.append("**To merge:** add the label `release-breaking-changes` (keep your other `release-*` label) and add this to the PR description:")
            lines.append("")
            lines.append("```markdown")
            lines.append("#### Upgrade steps")
            for s in verdict["UPGRADE_STEPS"] or ["<describe what a user must do before/after gcluster deploy>"]:
                lines.append(f"- {s}")
            lines.append("")
            lines.append("#### Redeploy impact")
            lines.append(verdict["REDEPLOY_IMPACT"] or "<one line; copied into the release notes>")
            lines.append("```")
        elif v == "NOT_BREAKING" and has_label:
            lines.append("")
            lines.append("The PR carries `release-breaking-changes` but the review found no breaking effect. If the label is intentional, keep it and fill in the description sections; otherwise remove it.")
        elif v == "NEEDS_PLAN_CHECK":
            lines.append("")
            lines.append("Please deploy the affected blueprint with the previous release, re-run `gcluster deploy` with this change, press `d`, and look for an error, `must be replaced`, or `will be destroyed`. "
                         "A maintainer can then add `breaking-check/plan-verified` (breaking confirmed) or `breaking-check/not-breaking`.")

    lines.append("")
    lines.append(f"<sub>Mode: `{mode}` · Definition and self-check: `skills/release-breaking-check/SKILL.md` · "
                 "Maintainers (not the author) may override with `breaking-check/not-breaking` or `breaking-check/plan-verified`.</sub>")
    return "\n".join(lines)


def gh_api(args: List[str], input_json: Optional[Dict] = None) -> str:
    cmd = ["gh", "api"] + args
    inp = json.dumps(input_json) if input_json is not None else None
    if inp is not None:
        cmd += ["--input", "-"]
    return subprocess.run(cmd, input=inp, capture_output=True, text=True, check=True).stdout


def upsert_comment(repo: str, pr_number: int, body: str) -> None:
    comments = json.loads(gh_api([f"repos/{repo}/issues/{pr_number}/comments", "--paginate"]))
    existing = next((c for c in comments if MARKER in (c.get("body") or "")), None)
    if existing:
        gh_api(["-X", "PATCH", f"repos/{repo}/issues/comments/{existing['id']}"], {"body": body})
    else:
        gh_api(["-X", "POST", f"repos/{repo}/issues/{pr_number}/comments"], {"body": body})


def main(argv: Optional[List[str]] = None) -> int:
    import argparse
    ap = argparse.ArgumentParser(description="Render and post the release-breaking check comment.")
    ap.add_argument("--verdict-file", help="file containing the skill output block (optional)")
    ap.add_argument("--scan-file", required=True, help="diff_scan.py JSON output")
    ap.add_argument("--pr-file", required=True, help="gh pr view --json number,labels,author output")
    ap.add_argument("--mode", default=os.environ.get("BREAKING_CHECK_MODE", "advisory"))
    ap.add_argument("--fork", action="store_true")
    ap.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY", ""))
    ap.add_argument("--dry-run", action="store_true", help="print the comment instead of posting")
    ap.add_argument("--out", help="write parsed verdict JSON here for the gate step")
    args = ap.parse_args(argv)

    scan = json.load(open(args.scan_file))
    pr_raw = json.load(open(args.pr_file))
    pr = {
        "number": pr_raw.get("number"),
        "labels": [l["name"] if isinstance(l, dict) else l for l in pr_raw.get("labels", [])],
        "author": (pr_raw.get("author") or {}).get("login", ""),
    }
    verdict = None
    if args.verdict_file and os.path.exists(args.verdict_file):
        verdict = parse_verdict(open(args.verdict_file, encoding="utf-8", errors="replace").read())
    if args.out:
        json.dump(verdict, open(args.out, "w"))

    body = render_comment(verdict, scan, pr, args.mode, fork=args.fork)
    if args.dry_run or not args.repo:
        print(body)
        return 0
    upsert_comment(args.repo, int(pr["number"]), body)
    print(f"Comment upserted on #{pr['number']} (verdict={verdict['VERDICT'] if verdict else None}).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
