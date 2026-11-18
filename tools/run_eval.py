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

"""Cluster Toolkit Agent Skills Evaluation and Linting CLI Entrypoint."""

import argparse
import contextlib
import json
import os
from pathlib import Path
import signal
import sys
import time
from typing import Any, Dict, Optional
import yaml

_REPO_ROOT = str(Path(__file__).resolve().parents[1])
if _REPO_ROOT not in sys.path:
    sys.path.insert(0, _REPO_ROOT)

from tools.skills_eval import (  # noqa: E402,F401
    ASSERTION_LIST_FIELDS,
    BLUEPRINT_HYGIENE_SCORERS,
    DEFAULT_SCORERS,
    EvalResult,
    EvaluatorBackend,
    FORBIDDEN_MUTATING_PATTERNS,
    FRONTMATTER_REGEX,
    LIVE_AGENT_COMMANDS,
    LIVE_AGENT_NOT_RUN_PATTERNS,
    LIVE_BLOCKED_LOG,
    LIVE_INVOCATIONS_LOG,
    LIVE_SHIM_DIR,
    LintResult,
    LiveCLIEvaluatorBackend,
    LiveOracles,
    MAX_LIVE_TOOL_CALLS,
    MockEvaluatorBackend,
    NAME_REGEX,
    REPO_ROOT,
    REQUIRED_FRONTMATTER,
    SCORER_REGISTRY,
    TF_PATH_REGEX,
    TestCaseResult,
    VALID_CASE_KEYS,
    VALID_STATUSES,
    VALID_SUITE_KEYS,
    _ACTION_HDR_RE,
    _BYPASS_CMD_RE,
    _CONFIRM_MSG,
    _EXEC_BYPASS_RE,
    _GCLOUD_SHIM,
    _GCLUSTER_SHIM,
    _GLOBAL_EVAL_TRIGGERS,
    _GLOBAL_EVAL_TRIGGER_PREFIXES,
    _GSUTIL_SHIM,
    _KUBECTL_SHIM,
    _MAKE_SHIM,
    _SECTION_TERM_RE,
    _TERRAFORM_SHIM,
    _check_skill_triggered,
    _code_block_lines,
    _coerce_to_string_list,
    _die,
    _extract_normalized_cases_and_summary,
    _find_fenced_forbidden_cmd,
    _has_confirmation,
    _parse_gemini_json_output,
    _scan_action_blocks,
    _strip_prose_for_tf_scan,
    _verify_assertions_categorized,
    _write_file,
    build_command_pattern,
    check_baseline_regression,
    check_command_safety,
    discover_skills,
    evaluate_skill,
    filter_cases,
    generate_comparison_markdown,
    generate_live_scorecard_markdown,
    lint_eval_yaml,
    lint_skill,
    normalize_case,
    parse_frontmatter,
    report_to_dict,
    resolve_active_scorers,
    resolve_case_expectations,
    resolve_case_forbidden_commands,
    resolve_case_scorers,
    resolve_changed_skills,
    sanitize_markdown_cell,
    skills_need_gcluster,
    verify_assertions,
    write_markdown_report,
)


def main() -> None:
    with contextlib.suppress(ValueError):
        signal.signal(signal.SIGTERM, lambda _s, _f: sys.exit(143))
    parser = argparse.ArgumentParser(description="Cluster Toolkit Agent Skills Evaluation Runner")
    for flag, kw in (
        ("--skill", {"type": str, "help": "Path(s) or name(s) of skill directory (comma-separated supported)"}),
        ("--all", {"action": "store_true", "help": "Run against all discovered skills"}),
        ("--skills-dir", {"type": str, "default": "skills", "help": "Root skills directory"}),
        ("--lint-only", {"action": "store_true", "help": "Run only static frontmatter linting"}),
        ("--cases", {"type": str, "default": None, "help": "Comma-separated case names/prefixes to run (or 'all' / 'smoke')"}),
        ("--json-output", {"type": str, "default": None, "help": "Path to write structured JSON evaluation report"}),
        ("--compare-json", {"nargs": "+", "metavar": "JSON_FILE", "help": "Compare 1 combined (before+after) JSON file or 2 (BEFORE AFTER) JSON files"}),
        ("--markdown-output", {"type": str, "help": "Path to write PR markdown summary table"}),
        ("--changed-from", {"type": str, "default": None, "help": "Git base ref to diff against HEAD and run only impacted skills"}),
        ("--check-needs-gcluster", {"action": "store_true", "help": "Exit 0 if any selected skill/case requires the gcluster binary, else exit 1"}),
    ):
        parser.add_argument(flag, **kw)  # type: ignore[arg-type]
    live = parser.add_argument_group("live evaluation (opt-in locally; run via Cloud Build PR smoke gate)")
    for flag, kw in (
        ("--live", {"action": "store_true", "help": "Run prompts against a real agent CLI in throwaway git worktrees instead of the offline mock"}),
        ("--agent", {"choices": sorted(LIVE_AGENT_COMMANDS), "default": "gemini", "help": "Agent CLI to drive with --live (default: gemini)"}),
        ("--project", {"type": str, "help": "GCP project substituted for 'my-proj' in prompts and used by gcluster validators (required with --live)"}),
        ("--repo-root", {"type": str, "default": None, "help": "Path to target git repository root for worktree creation and gcluster binary (default: repo root)"}),
        ("--repo-ref", {"type": str, "default": "HEAD", "help": "Git ref each live worktree is created from (default: HEAD)"}),
        ("--compare", {"type": str, "default": None, "help": "Baseline git ref ('Before') to evaluate and compare against --repo-ref ('After') in --live mode"}),
        ("--ablate-skill", {"action": "store_true", "help": "Compare 'Before' (skill and AGENTS.md removed at --repo-ref) against 'After' (with skill) in --live mode"}),
        ("--concurrency", {"type": int, "default": 1, "help": "Number of cases to evaluate in parallel in --live mode (default: 1)"}),
        ("--retries", {"type": int, "default": 0, "help": "Number of retries for a failing test case in --live mode (default: 0)"}),
        ("--timeout", {"type": int, "default": 600, "help": "Per-case agent timeout in seconds for --live (default: 600)"}),
        ("--keep-worktrees", {"action": "store_true", "help": "Do not delete live worktrees after each case (for debugging)"}),
        ("--eval-file", {"type": str, "help": "Run this cases file instead of the skill's EVAL.yaml (ad-hoc benchmarks; requires --skill)"}),
        ("--live-logs", {"type": str, "help": "Directory for per-case transcripts + oracles (default: live-logs/<agent>-<timestamp>)"}),
        ("--no-smoke-test", {"action": "store_true", "help": "Skip the pre-flight 'PONG' check of the agent CLI"}),
        ("--no-llm-judge", {"action": "store_true", "help": "Skip the LLM judge for expectations in --live mode"}),
    ):
        live.add_argument(flag, **kw)  # type: ignore[arg-type]
    args = parser.parse_args()
    if args.compare_json:
        if len(args.compare_json) not in (1, 2):
            _die("--compare-json accepts 1 combined JSON file or 2 (BEFORE AFTER) JSON files.")
        try:
            ld = [json.loads(Path(p).read_text(encoding="utf-8")) for p in args.compare_json]
            b_raw = (ld[0].get("before") or {}) if len(ld) == 1 else (
                ld[0]["before"] if isinstance(ld[0], dict) and "cases" not in ld[0] and isinstance(ld[0].get("before"), dict) else ld[0]
            )
            a_raw = (ld[0].get("after") or {}) if len(ld) == 1 else (
                ld[1]["after"] if isinstance(ld[1], dict) and "cases" not in ld[1] and isinstance(ld[1].get("after"), dict) else ld[1]
            )
        except Exception as e:  # noqa: BLE001
            _die(f"loading comparison JSON files: {e}")
        if not _extract_normalized_cases_and_summary(b_raw if isinstance(b_raw, dict) else {})[0] or not _extract_normalized_cases_and_summary(a_raw if isinstance(a_raw, dict) else {})[0]:
            _die("comparison JSON input has empty 'cases' in before or after payload.")
        scorecard_md = generate_comparison_markdown(b_raw, a_raw)
        print(scorecard_md)
        if args.markdown_output:
            _write_file(args.markdown_output, scorecard_md)
        reg_ok, _ = check_baseline_regression(b_raw, a_raw)
        sys.exit(0 if reg_ok else 1)
    repo_root = os.path.abspath(args.repo_root) if args.repo_root else (REPO_ROOT if args.skills_dir == "skills" else os.path.dirname(os.path.abspath(args.skills_dir)))
    eff_skills_dir = os.path.join(repo_root, "skills") if (args.repo_root and args.skills_dir == "skills") else args.skills_dir
    if args.skill:
        raw_skills = [s.strip() if os.path.isdir(s.strip()) else os.path.join(eff_skills_dir, s.strip()) for s in args.skill.split(",") if s.strip()]
    elif args.all:
        raw_skills = discover_skills(eff_skills_dir)
    elif args.changed_from:
        changed = resolve_changed_skills(args.changed_from, skills_dir=eff_skills_dir, repo_root=repo_root)
        if changed is None:
            raw_skills = discover_skills(eff_skills_dir)
        elif len(changed) == 0:
            if args.check_needs_gcluster:
                sys.exit(1)
            empty_rep = report_to_dict([], meta={"agent": args.agent, "project": args.project, "repo_ref": args.repo_ref, "live": bool(args.live), "skipped": True})
            if args.json_output:
                _write_file(args.json_output, json.dumps({"before": empty_rep, "after": empty_rep} if (args.live and (args.compare or args.ablate_skill)) else empty_rep, indent=2, sort_keys=True))
            if args.markdown_output:
                if args.live:
                    _write_file(args.markdown_output, generate_live_scorecard_markdown(empty_rep))
                else:
                    write_markdown_report([], args.markdown_output)
            print("[INFO] No skill-relevant changes detected; skipping evaluation.")
            sys.exit(0)
        else:
            raw_skills = changed
    else:
        raw_skills = []
    skills = list(dict.fromkeys(os.path.normpath(s) for s in raw_skills))
    for cond, err in (
        (bool(args.eval_file and not args.skill), "--eval-file requires --skill."),
        (args.concurrency < 1, "--concurrency must be >= 1."),
        (bool(args.compare and args.ablate_skill), "--compare and --ablate-skill are mutually exclusive."),
        (bool(args.live and args.lint_only), "--live cannot be combined with --lint-only."),
        (bool(args.live and not args.check_needs_gcluster and not args.project), "--live requires --project <gcp-project-id>."),
        (not skills, f"No skills found or specified (dir: {eff_skills_dir}). Use --skill, --all, or --changed-from."),
        (bool(args.eval_file and len(skills) != 1), "--eval-file requires a single --skill target."),
    ):
        if cond:
            _die(err)
    if args.check_needs_gcluster:
        try:
            sys.exit(0 if skills_need_gcluster(skills, args.cases, args.eval_file) else 1)
        except ValueError as e:
            _die(str(e))
    logs_dir = args.live_logs or os.path.join("live-logs", f"{args.agent}-{time.strftime('%Y%m%d-%H%M%S')}")
    all_passed, summary_rows, eval_results, valid_skills = True, [], [], []
    print(f"\n{'=' * 70}\nCluster Toolkit Skills Test Runner ({'LINT' if args.lint_only else ('LIVE EVAL' if args.live else 'EVAL')})\n{'=' * 70}")
    for s_path in skills:
        lint_res = lint_skill(s_path, eval_file=args.eval_file)
        if not lint_res.passed or args.lint_only:
            all_passed, st = (all_passed and lint_res.passed), ("PASS" if lint_res.passed else "FAIL")
            print(f"[{st}] {lint_res.skill_name} (Lint): {lint_res.message}")
            summary_rows.append({"skill": lint_res.skill_name, "type": "Lint", "status": st, "details": lint_res.message})
        else:
            valid_skills.append(s_path)
    if args.lint_only or not valid_skills:
        if args.markdown_output:
            write_markdown_report(summary_rows, args.markdown_output)
        print(f"\n{'=' * 70}\nResult: {'ALL CHECKS PASSED' if all_passed else 'FAILURES DETECTED'}\n{'=' * 70}\n")
        sys.exit(0 if all_passed else 1)
    is_multi = len(skills) > 1
    if args.cases:
        try:
            suites = [(sp, yaml.safe_load(Path(args.eval_file or os.path.join(sp, "EVAL.yaml")).read_text(encoding="utf-8")) or {}) for sp in valid_skills]
        except yaml.YAMLError as e:
            _die(str(e))
        for tok in (t.strip() for t in args.cases.split(",") if t.strip()):
            if not any(filter_cases((d.get("cases") or []) if isinstance(d, dict) else [], tok, skill_name=os.path.basename(os.path.normpath(sp)), allow_empty=True) for sp, d in suites):
                avail = ", ".join(
                    f"{os.path.basename(os.path.normpath(sp))}/{normalize_case(c, i + 1)['name']}"
                    for sp, d in suites if isinstance(d, dict)
                    for i, c in enumerate(d.get("cases") or []) if isinstance(c, dict)
                )
                _die(f"--cases filter token '{tok}' matched 0 cases. Available cases: {avail}")
        valid_skills = [
            sp for sp, d in suites
            if filter_cases((d.get("cases") or []) if isinstance(d, dict) else [], args.cases, skill_name=os.path.basename(os.path.normpath(sp)), allow_empty=True)
        ]
    req_gc, backend, live_kw = True, None, {}
    if args.live:
        try:
            req_gc = skills_need_gcluster(valid_skills, args.cases, args.eval_file)
        except ValueError as e:
            _die(str(e))
        live_kw = {
            "agent": args.agent, "project": args.project, "repo_root": repo_root, "timeout": args.timeout,
            "keep_worktrees": args.keep_worktrees, "llm_judge": not args.no_llm_judge, "require_gcluster": req_gc, "multi_skill": is_multi,
        }
        backend = LiveCLIEvaluatorBackend(**live_kw, repo_ref=args.repo_ref, logs_dir=logs_dir, overlay_skill=(args.repo_ref == "HEAD"))
        ok, why = backend.check_prerequisites(smoke_test=not args.no_smoke_test)
        if not ok:
            _die(why)
        print(
            f"[INFO] Per-case transcripts and oracles: {logs_dir}/\n"
            f"[INFO] Running LIVE against agent '{args.agent}' in project '{args.project}' (ref {args.repo_ref}, concurrency {args.concurrency}); "
            "verdicts come from filesystem/transcript oracles, keyword checks are informational.\n"
        )
    else:
        print("[INFO] Running in offline mock mode: validating assertion syntax and schema consistency.\n")

    def _eval_one(s_path: str, run_backend: Optional[EvaluatorBackend], conc: int, retry_cnt: int) -> EvalResult:
        try:
            return evaluate_skill(s_path, backend=run_backend, eval_file=args.eval_file, cases_filter=args.cases, concurrency=conc, retries=retry_cnt, allow_empty=is_multi)
        except ValueError as e:
            _die(str(e))

    before_report_dict: Optional[Dict[str, Any]] = None
    if args.live and (args.compare or args.ablate_skill):
        b_ref = args.compare or args.repo_ref
        print(f"[INFO] Running baseline ('Before') evaluation at ref '{b_ref}'{' (skill ablated)' if args.ablate_skill else ''} ...")
        before_backend = LiveCLIEvaluatorBackend(**live_kw, repo_ref=b_ref, logs_dir=os.path.join(logs_dir, "before"), ablate_skill=bool(args.ablate_skill), overlay_skill=False)
        before_report_dict = report_to_dict(
            [_eval_one(s_path, before_backend, args.concurrency, args.retries if args.live else 0) for s_path in valid_skills],
            backend=before_backend,
            meta={"agent": args.agent, "project": args.project, "repo_ref": b_ref, "role": "before", "ablate_skill": bool(args.ablate_skill)},
        )
    for s_path in valid_skills:
        eval_res = _eval_one(s_path, backend, args.concurrency if args.live else 1, args.retries if args.live else 0)
        eval_results.append(eval_res)
        failed_cases, all_passed = [tc for tc in eval_res.cases if not tc.passed], (all_passed and eval_res.passed)
        print(f"[PASS] {eval_res.skill_name} (Eval): All {len(eval_res.cases)} cases passed" if eval_res.passed else f"[FAIL] {eval_res.skill_name} (Eval): {eval_res.message}")
        for tc in failed_cases:
            print(f"  - [FAIL] Case '{tc.case_name}' ({tc.latency_seconds}s)\n    Reason: {tc.message}")
        if not eval_res.passed and len(eval_res.cases) > len(failed_cases):
            print(f"  - [PASS] {len(eval_res.cases) - len(failed_cases)} other case(s) passed")
        summary_rows.extend({"skill": eval_res.skill_name, "type": f"Case: {tc.case_name}", "status": "PASS" if tc.passed else "FAIL", "details": tc.message} for tc in eval_res.cases)
    after_report_dict = report_to_dict(eval_results, backend=backend, meta={"agent": args.agent, "project": args.project, "repo_ref": args.repo_ref, "live": bool(args.live)})
    if args.compare and before_report_dict is not None:
        reg_ok, regs = check_baseline_regression(before_report_dict, after_report_dict)
        if not reg_ok:
            all_passed = False
            for r in regs:
                print(f"[REGRESSION] {r}")
    if args.json_output:
        _write_file(args.json_output, json.dumps({"before": before_report_dict, "after": after_report_dict} if before_report_dict is not None else after_report_dict, indent=2, sort_keys=True))
    if before_report_dict is not None or args.live:
        out_md = generate_comparison_markdown(before_report_dict, after_report_dict) if before_report_dict is not None else generate_live_scorecard_markdown(after_report_dict)
        print(f"\n{out_md}")
        if args.markdown_output:
            _write_file(args.markdown_output, out_md)
    elif args.markdown_output:
        write_markdown_report(summary_rows, args.markdown_output)
    print(f"\n{'=' * 70}\nResult: {'ALL CHECKS PASSED' if all_passed else 'FAILURES DETECTED'}\n{'=' * 70}\n")
    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    main()
