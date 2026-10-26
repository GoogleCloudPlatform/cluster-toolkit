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

"""Gate for the release-breaking check: compares verdict with labels/description.

Rules (exit 1 on failure when MODE=enforce; warnings only when MODE=advisory):
  1. verdict BREAKING and label `release-breaking-changes` missing        -> fail
  2. label present and PR body lacks non-empty Upgrade steps / Redeploy impact -> fail
  3. verdict NEEDS_PLAN_CHECK and no valid override label                 -> fail
  4. an override label (`breaking-check/not-breaking`, `breaking-check/plan-verified`)
     is valid only if the actor who added it is a maintainer listed in
     cluster-toolkit-writers.json and is not the PR author.
Standard library only.
"""

import json
import os
import re
import subprocess
import sys
from typing import Dict, List, Optional, Set, Tuple

BREAKING_LABEL = "release-breaking-changes"
OVERRIDE_NOT_BREAKING = "breaking-check/not-breaking"
OVERRIDE_PLAN_VERIFIED = "breaking-check/plan-verified"
OVERRIDES = (OVERRIDE_NOT_BREAKING, OVERRIDE_PLAN_VERIFIED)


def section_text(body: str, heading: str) -> str:
    """Text under `#### <heading>` up to the next heading, minus HTML comments and whitespace."""
    body = (body or "").replace("\r", "")
    lines = body.split("\n")
    out: List[str] = []
    on = False
    for ln in lines:
        if ln.strip() == f"#### {heading}":
            on = True
            continue
        if on and re.match(r"^#{1,4} ", ln):
            break
        if on:
            out.append(ln)
    txt = re.sub(r"<!--.*?-->", "", "\n".join(out), flags=re.DOTALL)
    return re.sub(r"\s+", "", txt)


def load_maintainers(path: str) -> Set[str]:
    try:
        data = json.load(open(path))
    except (OSError, ValueError):
        return set()
    return {str(e.get("login", "")).lower() for e in data if isinstance(e, dict)}


def valid_overrides(label_events: List[Dict], maintainers: Set[str], author: str) -> Dict[str, str]:
    """Return {override_label: actor} for overrides added by a maintainer other than the author.

    label_events: items from GET /repos/{r}/issues/{n}/timeline filtered to event == 'labeled',
    in chronological order. A later 'unlabeled' removes it.
    """
    state: Dict[str, str] = {}
    for ev in label_events:
        name = ((ev.get("label") or {}).get("name") or "")
        if name not in OVERRIDES:
            continue
        if ev.get("event") == "unlabeled":
            state.pop(name, None)
            continue
        actor = ((ev.get("actor") or {}).get("login") or "").lower()
        if actor and actor in maintainers and actor != (author or "").lower():
            state[name] = actor
        else:
            state[name] = ""  # present but not trusted
    return {k: v for k, v in state.items() if v}


def evaluate(verdict: Optional[Dict], labels: List[str], body: str, overrides: Dict[str, str],
             untrusted_overrides: List[str]) -> Tuple[List[str], List[str]]:
    """Return (failures, notes)."""
    failures: List[str] = []
    notes: List[str] = []
    has_label = BREAKING_LABEL in labels
    v = (verdict or {}).get("VERDICT")

    if untrusted_overrides:
        notes.append("Override label(s) ignored because they were not added by a maintainer other than the PR author: "
                     + ", ".join(untrusted_overrides))

    if OVERRIDE_NOT_BREAKING in overrides:
        notes.append(f"Override `{OVERRIDE_NOT_BREAKING}` accepted (added by @{overrides[OVERRIDE_NOT_BREAKING]}).")
        v = "NOT_BREAKING"
    elif OVERRIDE_PLAN_VERIFIED in overrides:
        notes.append(f"Override `{OVERRIDE_PLAN_VERIFIED}` accepted (added by @{overrides[OVERRIDE_PLAN_VERIFIED]}).")
        v = "BREAKING"

    if v == "BREAKING" and not has_label:
        failures.append(f"Review verdict is BREAKING but the PR lacks the `{BREAKING_LABEL}` label.")
    if v == "NEEDS_PLAN_CHECK":
        failures.append("Review could not settle the verdict (NEEDS_PLAN_CHECK). Run the old->new plan check and have a "
                        f"maintainer add `{OVERRIDE_PLAN_VERIFIED}` or `{OVERRIDE_NOT_BREAKING}`.")
    if has_label:
        for heading in ("Upgrade steps", "Redeploy impact"):
            if not section_text(body, heading):
                failures.append(f"PR carries `{BREAKING_LABEL}` but the description's `#### {heading}` section is empty.")
    if v == "NOT_BREAKING" and has_label and OVERRIDE_NOT_BREAKING not in overrides:
        notes.append("Verdict NOT_BREAKING but the breaking label is present; keeping the label is allowed (author's call).")
    return failures, notes


def fetch_label_events(repo: str, pr_number: int) -> List[Dict]:
    out = subprocess.run(
        ["gh", "api", "--paginate", "-H", "Accept: application/vnd.github+json",
         f"repos/{repo}/issues/{pr_number}/timeline?per_page=100"],
        capture_output=True, text=True, check=True).stdout
    # --paginate may concatenate arrays; normalise.
    events: List[Dict] = []
    for chunk in re.findall(r"\[.*?\](?=\s*\[|\s*$)", out, flags=re.DOTALL) or [out]:
        try:
            events.extend(json.loads(chunk))
        except ValueError:
            continue
    return [e for e in events if e.get("event") in ("labeled", "unlabeled")]


def main(argv: Optional[List[str]] = None) -> int:
    import argparse
    ap = argparse.ArgumentParser(description="Release-breaking check gate.")
    ap.add_argument("--verdict-json", help="parsed verdict JSON from breaking_check_comment.py --out (optional)")
    ap.add_argument("--pr-file", required=True, help="gh pr view --json number,labels,author,body")
    ap.add_argument("--maintainers", default="cluster-toolkit-writers.json")
    ap.add_argument("--mode", default=os.environ.get("BREAKING_CHECK_MODE", "advisory"))
    ap.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY", ""))
    ap.add_argument("--events-file", help="pre-fetched timeline events JSON (for tests)")
    args = ap.parse_args(argv)

    pr = json.load(open(args.pr_file))
    labels = [l["name"] if isinstance(l, dict) else l for l in pr.get("labels", [])]
    author = (pr.get("author") or {}).get("login", "")
    body = pr.get("body") or ""
    verdict = None
    if args.verdict_json and os.path.exists(args.verdict_json):
        try:
            verdict = json.load(open(args.verdict_json))
        except ValueError:
            verdict = None

    maintainers = load_maintainers(args.maintainers)
    if args.events_file:
        events = json.load(open(args.events_file))
    elif args.repo and any(l in OVERRIDES for l in labels):
        events = fetch_label_events(args.repo, int(pr["number"]))
    else:
        events = []
    trusted = valid_overrides(events, maintainers, author)
    untrusted = [l for l in labels if l in OVERRIDES and l not in trusted]

    failures, notes = evaluate(verdict, labels, body, trusted, untrusted)
    for n in notes:
        print(f"::notice::{n}")
    if not failures:
        print(f"Release-breaking check passed (verdict={(verdict or {}).get('VERDICT')}, mode={args.mode}).")
        return 0
    level = "error" if args.mode == "enforce" else "warning"
    for f in failures:
        print(f"::{level}::{f}")
    if args.mode == "enforce":
        print("Release-breaking check FAILED. See the 'Release-breaking check' comment on the PR for suggested text.")
        return 1
    print("Release-breaking check: advisory mode, not failing the build.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
