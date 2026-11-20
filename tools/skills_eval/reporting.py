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

"""Skill evaluation orchestration, git diff target discovery, and scorecard reporting."""

import concurrent.futures
from dataclasses import asdict
import html
import os
from pathlib import Path
import subprocess
import sys
import time
from typing import Any, Dict, List, Optional, Set, Tuple
import yaml

from .live_backend import LiveCLIEvaluatorBackend, LiveOracles
from .plan_parser import (
    EvaluatorBackend,
    MockEvaluatorBackend,
    _CONFIRM_MSG,
    _find_fenced_forbidden_cmd,
    _has_confirmation,
    _verify_assertions_categorized,
)
from .schema_lint import (
    EvalResult,
    MAX_LIVE_TOOL_CALLS,
    REPO_ROOT,
    TestCaseResult,
    _write_file,
    filter_cases,
    normalize_case,
    resolve_active_scorers,
)


def evaluate_skill(
    skill_path: str,
    backend: Optional[EvaluatorBackend] = None,
    eval_file: Optional[str] = None,
    cases_filter: Optional[str] = None,
    concurrency: int = 1,
    case_filter: Optional[List[str]] = None,
    retries: int = 0,
    allow_empty: bool = False,
) -> EvalResult:
    """Evaluate skill test cases deterministically (eval_file overrides <skill_path>/EVAL.yaml)."""
    skill_name = os.path.basename(os.path.normpath(skill_path))
    abs_skill_path = os.path.abspath(skill_path)
    eval_yaml_path = eval_file or os.path.join(skill_path, "EVAL.yaml")
    if not os.path.isfile(eval_yaml_path):
        return EvalResult(skill_name, False, f"Missing {eval_yaml_path}")
    try:
        eval_data = yaml.safe_load(Path(eval_yaml_path).read_text(encoding="utf-8"))
    except yaml.YAMLError as e:
        return EvalResult(skill_name, False, f"EVAL.yaml parse error: {e}")
    if not isinstance(eval_data, dict) or not isinstance(eval_data.get("cases"), list) or not eval_data["cases"]:
        return EvalResult(skill_name, False, "EVAL.yaml must contain non-empty 'cases' list")
    selected_cases = filter_cases(
        eval_data["cases"],
        cases_filter or (",".join(case_filter) if case_filter else None),
        skill_name=skill_name,
        allow_empty=allow_empty,
    )
    backend = backend or MockEvaluatorBackend()

    def _run_one(item: Tuple[int, Any]) -> Tuple[int, TestCaseResult]:
        idx, raw_case = item
        if not isinstance(raw_case, dict):
            return idx, TestCaseResult(f"case_{idx + 1}", False, f"Case entry must be a dictionary, got {type(raw_case).__name__}")
        case = normalize_case(raw_case, index=idx + 1, suite_data=eval_data)
        cname, prompt, qname = case["name"], case["prompt"], f"{skill_name}/{case['name']}"
        active_sc = set(case.get("_active_scorers") or resolve_active_scorers(eval_data, raw_case))
        ok, msg, elapsed = False, "", 0.0
        prev_toks = {k: 0 for k in ("input_tokens", "output_tokens", "cached_tokens", "thought_tokens", "total_tokens", "judge_tokens")}
        for attempt in range(1, max(0, retries) + 2):
            attempt_start = time.time()
            simulated_text = (
                backend.run_case(skill_name, prompt, case, skill_path=abs_skill_path)
                if isinstance(backend, LiveCLIEvaluatorBackend)
                else backend.run_case(skill_name, prompt, case)
            )
            elapsed = round(elapsed + (time.time() - attempt_start), 4)
            ok, msg, fail_cat = _verify_assertions_categorized(simulated_text, case)
            orc_cur = backend._get_oracle(cname, skill_name) if isinstance(backend, LiveCLIEvaluatorBackend) else None
            if orc_cur is not None:
                orc_cur.attempts = attempt
                for tk in prev_toks:
                    setattr(orc_cur, tk, getattr(orc_cur, tk) + prev_toks[tk])
                    prev_toks[tk] = getattr(orc_cur, tk)
            hard_fn = getattr(backend, "hard_failures", None)
            if callable(hard_fn):
                hard = list(hard_fn(qname if isinstance(backend, LiveCLIEvaluatorBackend) and qname in backend.oracles else cname))
                llm_judged = bool("expectations" in active_sc and case.get("expectations") and getattr(backend, "llm_judge", False))
                fenced_err = _find_fenced_forbidden_cmd(simulated_text, case) if llm_judged else None
                if fenced_err and fenced_err not in hard:
                    hard.append(fenced_err)
                det_err = (
                    msg
                    if (
                        not ok
                        and (
                            fail_cat in ("missing_header", "missing_confirm")
                            or (not llm_judged and fail_cat == "forbidden_cmd")
                            or (
                                not llm_judged
                                and not ("script_validators" in active_sc and case.get("script_validators"))
                                and isinstance(backend, LiveCLIEvaluatorBackend)
                            )
                        )
                    )
                    else (_CONFIRM_MSG if (llm_judged and case.get("expect_blocked_action") and not _has_confirmation(simulated_text or "")) else "")
                )
                if det_err and det_err not in hard:
                    hard.append(det_err)
                ok, msg = not hard, (("; ".join(hard) if hard else "All hard oracles passed") + f" ({'keywords ok' if ok else 'keywords: ' + msg})")
            if ok or (
                orc_cur is not None
                and (
                    orc_cur.blocked_commands
                    or orc_cur.tracked_template_edits
                    or orc_cur.generated_file_edits
                    or orc_cur.shim_replaced
                    or orc_cur.agent_not_run
                    or orc_cur.timed_out
                    or orc_cur.agent_exit_code not in (None, 0)
                    or orc_cur.tool_calls_total > MAX_LIVE_TOOL_CALLS
                )
            ):
                break
        if isinstance(backend, LiveCLIEvaluatorBackend):
            backend._sync_case_log_verdict(cname, ok, msg, skill_name=skill_name)
            oracles = backend._get_oracle(cname, skill_name) or LiveOracles()
            with backend._print_lock:
                print(
                    f"  [DONE] {cname}: {'PASS' if ok else 'FAIL'} "
                    f"({elapsed:.1f}s, tokens={oracles.total_tokens}, tools={oracles.tool_calls_total}, tf_read={oracles.tf_files_read})"
                )
                if not ok:
                    print(f"    [FAIL-REASON] {cname}: {msg}\n    [FAIL-ORACLES] {cname}: {oracles.summary()}")
                    log_path = backend._case_log_path(cname, skill_name)
                    if log_path and os.path.isfile(log_path):
                        tail = Path(log_path).read_text(encoding="utf-8", errors="replace").splitlines()[-40:]
                        print(f"    [FAIL-LOG-TAIL] {cname} ({log_path}, last {len(tail)} lines):\n" + "\n".join(f"      {t}" for t in tail))
                sys.stdout.flush()
        return idx, TestCaseResult(cname, ok, msg, latency_seconds=elapsed)

    indexed = list(enumerate(selected_cases))
    if concurrency > 1 and len(indexed) > 1:
        with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as pool:
            results = [tc for _, tc in sorted(pool.map(_run_one, indexed), key=lambda p: p[0])]
    else:
        results = [_run_one(item)[1] for item in indexed]
    failed_cases = [tc for tc in results if not tc.passed]
    return EvalResult(
        skill_name,
        not failed_cases,
        f"{len(failed_cases)} of {len(results)} cases failed" if failed_cases else f"All {len(results)} cases passed",
        results,
    )


def discover_skills(skills_dir: str) -> List[str]:
    """Recursively discover all directories containing SKILL.md, ignoring hidden directories."""
    skills: List[str] = []
    for root, dirs, files in (os.walk(skills_dir) if os.path.isdir(skills_dir) else ()):
        dirs[:] = [d for d in dirs if not d.startswith(".")]
        if "SKILL.md" in files:
            skills.append(root)
    return sorted(skills)


_GLOBAL_EVAL_TRIGGERS = frozenset({
    "AGENTS.md",
    "CLAUDE.md",
    "GEMINI.md",
    "llms.txt",
    "llms-full.txt",
    "tools/run_eval.py",
    "tools/tests/test_run_eval.py",
    "tools/validate_readme_snippets.py",
    "tools/cloud-build/pr-skills-live-eval.yaml",
    "tools/cloud-build/provision/pr-skills-live-eval.tf",
})
_GLOBAL_EVAL_TRIGGER_PREFIXES = (
    "tools/skills_eval/",
    "tools/cloud-build/images/test-runner/",
)


def resolve_changed_skills(base_ref: str, skills_dir: str = "skills", repo_root: Optional[str] = None) -> Optional[List[str]]:
    """Return sorted skill paths impacted since base_ref, [] if none, or None to fall back to all skills."""
    ref = (base_ref or "").strip()
    if not ref:
        return None
    root = os.path.abspath(repo_root or REPO_ROOT)
    skills_root_abs = os.path.abspath(skills_dir if os.path.isabs(skills_dir) else os.path.join(root, skills_dir))
    rel_prefix = os.path.relpath(skills_root_abs, root).replace("\\", "/").strip("/")
    if rel_prefix.startswith("..") or not rel_prefix or rel_prefix == ".":
        skills_prefix = "skills/"
    else:
        skills_prefix = f"{rel_prefix}/"

    def _git_q(gargs: List[str]) -> "subprocess.CompletedProcess[str]":
        return subprocess.run(["git", *gargs], cwd=root, capture_output=True, text=True, check=False)

    base_p = _git_q(["rev-parse", "--verify", f"{ref}^{{commit}}"])
    head_p = _git_q(["rev-parse", "--verify", "HEAD^{commit}"])
    if base_p.returncode != 0 or head_p.returncode != 0 or base_p.stdout.strip() == head_p.stdout.strip():
        return None
    mb = _git_q(["merge-base", ref, "HEAD"])
    diff = _git_q(
        ["diff", "--name-only", mb.stdout.strip(), "HEAD"]
        if (mb.returncode == 0 and mb.stdout.strip())
        else ["diff", "--name-only", ref, "HEAD"]
    )
    if diff.returncode != 0:
        return None
    changed = [ln.strip().replace("\\", "/") for ln in diff.stdout.splitlines() if ln.strip()]
    if not changed:
        return None
    impacted: Set[str] = set()
    for p in changed:
        if p in (
            f"{skills_prefix}README.md",
            "skills/README.md",
            "tools/skills_eval/README.md",
            "tools/cloud-build/provision/README.md",
        ) or (
            p.startswith(skills_prefix) and p.endswith("/references/catalog.yaml")
        ):
            continue
        if p in _GLOBAL_EVAL_TRIGGERS or p.startswith(_GLOBAL_EVAL_TRIGGER_PREFIXES):
            return None
        if p == "docs/migration/xpk_to_clustertoolkit.md":
            xpk_abs = os.path.join(skills_root_abs, "xpk-to-clustertoolkit")
            if os.path.isfile(os.path.join(xpk_abs, "SKILL.md")):
                impacted.add(os.path.normpath(os.path.join(skills_dir, "xpk-to-clustertoolkit")))
                continue
            return None
        if p.startswith(skills_prefix):
            cur = os.path.abspath(os.path.dirname(os.path.join(root, p)))
            found = ""
            while cur.startswith(skills_root_abs + os.sep):
                if os.path.isfile(os.path.join(cur, "SKILL.md")):
                    found = os.path.normpath(os.path.join(skills_dir, os.path.relpath(cur, skills_root_abs)))
                    break
                parent = os.path.dirname(cur)
                if parent == cur:
                    break
                cur = parent
            if not found:
                return None
            impacted.add(found)
    return sorted(impacted)


def skills_need_gcluster(skill_paths: List[str], cases_arg: Optional[str] = None, eval_file: Optional[str] = None) -> bool:
    """Return True if any selected case across skill_paths activates the blueprint_reexpand scorer."""
    for sp in skill_paths:
        ep = eval_file or os.path.join(sp, "EVAL.yaml")
        if not os.path.isfile(ep):
            continue
        try:
            data = yaml.safe_load(Path(ep).read_text(encoding="utf-8")) or {}
        except yaml.YAMLError:
            continue
        if not isinstance(data, dict):
            continue
        sname = os.path.basename(os.path.normpath(sp))
        for rc in filter_cases(data.get("cases") or [], cases_arg, skill_name=sname, allow_empty=True):
            if isinstance(rc, dict) and "blueprint_reexpand" in resolve_active_scorers(data, rc):
                return True
    return False


def sanitize_markdown_cell(value: str) -> str:
    """Sanitize string for safe embedding into a GitHub Markdown table cell."""
    escaped = html.escape((value or "").replace("\r\n", "\n").replace("\r", "\n"), quote=True)
    return escaped.replace("\n", "<br>").replace("|", "&#124;").strip() if value else ""


def write_markdown_report(summary_rows: List[Dict[str, str]], output_path: str) -> None:
    """Write sanitized GitHub Actions markdown summary table."""
    lines = [
        "### Cluster Toolkit Skills Evaluation Results\n",
        "| Skill | Test / Check | Status | Details |",
        "| :--- | :--- | :--- | :--- |",
        *[
            f"| `{sanitize_markdown_cell(r['skill'])}` | {sanitize_markdown_cell(r['type'])} | "
            f"{'PASS' if r['status'] == 'PASS' else 'FAIL'} | {sanitize_markdown_cell(r['details'])} |"
            for r in summary_rows
        ],
    ]
    _write_file(output_path, "\n".join(lines) + "\n")


def _tf_count(orc: Optional[Dict[str, Any]]) -> int:
    val = (orc or {}).get("tf_files_read", 0)
    return len(val) if isinstance(val, list) else int(val or 0)


def _extract_normalized_cases_and_summary(data: Dict[str, Any]) -> Tuple[Dict[str, Dict[str, Any]], Dict[str, Any]]:
    raw = data.get("cases") or {}
    items = (
        [(e.get("case_name") or e.get("name") or f"case_{i + 1}", e) for i, e in enumerate(raw) if isinstance(e, dict)]
        if isinstance(raw, list)
        else [(k, v) for k, v in raw.items() if isinstance(v, dict)]
        if isinstance(raw, dict)
        else []
    )
    cases_map: Dict[str, Dict[str, Any]] = {str(k): v for k, v in items}
    vals = list(cases_map.values())
    orcs = [e["oracles"] if isinstance(e.get("oracles"), dict) else e for e in vals]
    tot, denom, passed = len(vals), max(len(vals), 1), sum(1 for e in vals if e.get("passed"))
    tot_tok = sum(int(o.get("total_tokens", 0) or 0) for o in orcs)
    tot_tools = sum(int(o.get("tool_calls_total", 0) or 0) for o in orcs)
    tot_lat = sum(float(e.get("latency_seconds", 0.0) or 0.0) for e in vals)
    summary: Dict[str, Any] = {
        "total_cases": tot,
        "passed_cases": passed,
        "pass_rate": round((passed / denom) * 100.0, 2) if tot else 0.0,
        "total_tf_files_read": sum(_tf_count(o) for o in orcs),
        "reexpand_ok_count": sum(1 for o in orcs if str(o.get("reexpand", "")).startswith("ok")),
        "skills_triggered": sum(1 for o in orcs if o.get("skill_trigger_relevant") and o.get("skill_triggered", True)),
        "has_tf": any("no_tf_read" in (o.get("active_scorers") or ["no_tf_read", "blueprint_reexpand"]) for o in orcs) if orcs else True,
        "has_reexpand": any("blueprint_reexpand" in (o.get("active_scorers") or ["no_tf_read", "blueprint_reexpand"]) for o in orcs) if orcs else True,
        "has_trig": any(bool(o.get("skill_trigger_relevant")) for o in orcs),
        "total_input_tokens": sum(int(o.get("input_tokens", 0) or 0) for o in orcs),
        "total_output_tokens": sum(int(o.get("output_tokens", 0) or 0) for o in orcs),
        "total_cached_tokens": sum(int(o.get("cached_tokens", 0) or 0) for o in orcs),
        "total_thought_tokens": sum(int(o.get("thought_tokens", 0) or 0) for o in orcs),
        "total_tokens": tot_tok,
        "mean_tokens_per_case": round(tot_tok / denom, 1) if tot else 0.0,
        "total_judge_tokens": sum(int(o.get("judge_tokens", 0) or 0) for o in orcs),
        "total_tool_calls": tot_tools,
        "mean_tool_calls_per_case": round(tot_tools / denom, 2) if tot else 0.0,
        "total_latency_seconds": round(tot_lat, 2),
        "mean_latency_seconds": round(tot_lat / denom, 2) if tot else 0.0,
    }
    if isinstance(data.get("summary"), dict):
        summary.update(data["summary"])
    return cases_map, summary


def report_to_dict(eval_results: List[EvalResult], backend: Optional[EvaluatorBackend] = None, meta: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    oracles_map = getattr(backend, "oracles", {}) if backend is not None else {}
    full_meta: Dict[str, Any] = {"timestamp_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}
    if isinstance(backend, LiveCLIEvaluatorBackend):
        sha = backend._git(["rev-parse", backend.repo_ref], backend.repo_root).stdout.strip()
        full_meta.update({"commit_sha": sha} if sha else {})
    full_meta.update(meta or {})
    multi, cases_map = len(eval_results) > 1, {}
    for res in eval_results:
        for tc in res.cases:
            base_k = f"{res.skill_name}/{tc.case_name}" if multi else tc.case_name
            k, n = base_k, 2
            while k in cases_map:
                k, n = f"{base_k}#{n}", n + 1
            orc = oracles_map.get(f"{res.skill_name}/{tc.case_name}") or oracles_map.get(tc.case_name)
            cases_map[k] = {
                "case_name": tc.case_name,
                "skill_name": res.skill_name,
                "passed": tc.passed,
                "message": tc.message,
                "latency_seconds": tc.latency_seconds,
                "oracles": asdict(orc) if orc else {},
            }
    return {
        "metadata": full_meta,
        "skills": [{"skill_name": r.skill_name, "passed": r.passed, "message": r.message} for r in eval_results],
        "cases": cases_map,
        "summary": _extract_normalized_cases_and_summary({"cases": cases_map})[1],
    }


_SCORECARD_METRICS = (
    ("Total .tf Files Read", "total_tf_files_read", "{}", "0"),
    ("Successful Blueprint Re-expansions", "reexpand_ok_count", "{}", "all blueprint/deployment cases"),
    ("Total Input Tokens", "total_input_tokens", "{:,}", "—"),
    ("Total Output Tokens", "total_output_tokens", "{:,}", "—"),
    ("Total Cached Tokens", "total_cached_tokens", "{:,}", "—"),
    ("Total Thought Tokens", "total_thought_tokens", "{:,}", "—"),
    ("Total Tokens (All Cases)", "total_tokens", "{:,}", "<= 1.2M / case max"),
    ("Mean Tokens / Case", "mean_tokens_per_case", "{:,.1f}", "<= 500,000"),
    ("Total Tool Calls", "total_tool_calls", "{}", "<= 45 / case max"),
    ("Mean Tool Calls / Case", "mean_tool_calls_per_case", "{:.2f}", "<= 25"),
    ("Total Agent Wall-Clock Time (s)", "total_latency_seconds", "{:.1f}s", "<= 300s / case max"),
    ("Mean Latency / Case (s)", "mean_latency_seconds", "{:.1f}s", "<= 150s"),
)


def _active_scorecard_metrics(show_tf: bool, show_reexpand: bool, show_trig: bool) -> List[Tuple[str, str, str, str]]:
    out = [m for m in _SCORECARD_METRICS if (m[1] != "total_tf_files_read" or show_tf) and (m[1] != "reexpand_ok_count" or show_reexpand)]
    return ([("Skills Triggered (SKILL.md Read)", "skills_triggered", "{}", "all non-ablated skill cases")] if show_trig else []) + out


def generate_live_scorecard_markdown(report_data: Dict[str, Any]) -> str:
    cases_map, s = _extract_normalized_cases_and_summary(report_data)
    show_tf, show_re, show_trig = bool(s.get("has_tf", True)), bool(s.get("has_reexpand", True)), bool(s.get("has_trig", False))
    cols = [
        "Case", "Status", *(["Skill Read"] if show_trig else []), *([".tf Read"] if show_tf else []),
        *(["Re-expand"] if show_re else []), "Tokens", "Tool Calls", "Time (s)", "Verdict / Judge Summary",
    ]
    lines = [
        "## Cluster Toolkit Agent Live Evaluation Scorecard", "", "### 1. Executive Summary Scorecard", "",
        "| Metric | Value | Target |", "| :--- | :--- | :--- |",
        f"| **Pass Rate** | {s['passed_cases']}/{s['total_cases']} ({s['pass_rate']:.1f}%) | 100% |",
        *[f"| **{label}** | {fmt.format(s[key])} | {target} |" for label, key, fmt, target in _active_scorecard_metrics(show_tf, show_re, show_trig)],
        "", "### 2. Per-Case Scorecard Table", "", "| " + " | ".join(cols) + " |", "| " + " | ".join(":---" for _ in cols) + " |",
    ]
    for cname, c in cases_map.items():
        orc = c["oracles"] if isinstance(c.get("oracles"), dict) else c
        reexp = str(orc.get("reexpand", "skipped") or "skipped").split(":", 1)[0]
        s_read = ("yes" if orc.get("skill_triggered", True) else "no") if orc.get("skill_trigger_relevant") else "n/a"
        verdict = sanitize_markdown_cell("; ".join(orc.get("judge_reasoning") or []) or str(c.get("message", "")))
        row = [
            f"`{sanitize_markdown_cell(cname)}`", "PASS" if c.get("passed") else "FAIL",
            *([s_read] if show_trig else []), *([str(_tf_count(orc))] if show_tf else []), *([sanitize_markdown_cell(reexp)] if show_re else []),
            f"{int(orc.get('total_tokens', 0) or 0):,}", str(int(orc.get("tool_calls_total", 0) or 0)),
            f"{float(c.get('latency_seconds', 0.0) or 0.0):.1f}s", verdict,
        ]
        lines.append("| " + " | ".join(row) + " |")
    return "\n".join(lines) + "\n"


def _align_comparison_keys(
    b_cases: Dict[str, Dict[str, Any]], a_cases: Dict[str, Dict[str, Any]]
) -> Tuple[Dict[str, Dict[str, Any]], Dict[str, Dict[str, Any]]]:
    if not any("/" in k for k in (*b_cases, *a_cases)):
        return b_cases, a_cases

    def _remap(src: Dict[str, Dict[str, Any]], peer: Dict[str, Dict[str, Any]]) -> Dict[str, Dict[str, Any]]:
        aligned: Dict[str, Dict[str, Any]] = {}
        for key, entry in src.items():
            if "/" in key:
                aligned[key] = entry
            elif entry.get("skill_name"):
                aligned[f"{entry['skill_name']}/{key}"] = entry
            else:
                suffix = f"/{key}"
                peer_hits = [pk for pk in peer if pk.endswith(suffix)]
                aligned[peer_hits[0] if len(peer_hits) == 1 else key] = entry
        return aligned

    return _remap(b_cases, a_cases), _remap(a_cases, b_cases)


def check_baseline_regression(before_data: Dict[str, Any], after_data: Dict[str, Any]) -> Tuple[bool, List[str]]:
    """Verify 'After' has zero case pass->fail regressions relative to 'Before'."""
    b_cases, _ = _extract_normalized_cases_and_summary(before_data)
    a_cases, _ = _extract_normalized_cases_and_summary(after_data)
    b_cases, a_cases = _align_comparison_keys(b_cases, a_cases)
    regressions = [
        f"{cname}: regressed from PASS to {'FAIL' if cname in a_cases else 'MISSING'}"
        for cname, bc in b_cases.items()
        if bc.get("passed") and not a_cases.get(cname, {}).get("passed")
    ]
    return not regressions, regressions


def generate_comparison_markdown(before_data: Dict[str, Any], after_data: Dict[str, Any]) -> str:
    b_cases, b_sum = _extract_normalized_cases_and_summary(before_data)
    a_cases, a_sum = _extract_normalized_cases_and_summary(after_data)
    b_cases, a_cases = _align_comparison_keys(b_cases, a_cases)
    show_tf = bool(b_sum.get("has_tf", True) or a_sum.get("has_tf", True))
    show_re = bool(b_sum.get("has_reexpand", True) or a_sum.get("has_reexpand", True))
    show_trig = bool(b_sum.get("has_trig", False) or a_sum.get("has_trig", False))
    rate_diff = a_sum["pass_rate"] - b_sum["pass_rate"]

    def _exec_row(label: str, key: str, fmt: str) -> str:
        if key == "skills_triggered" and not (b_sum.get("has_trig") and a_sum.get("has_trig")):
            return f"| **{label}** | {fmt.format(b_sum[key]) if b_sum.get('has_trig') else 'n/a'} | {fmt.format(a_sum[key]) if a_sum.get('has_trig') else 'n/a'} | n/a |"
        diff = a_sum[key] - b_sum[key]
        pct = ((a_sum[key] - b_sum[key]) / b_sum[key]) * 100.0 if b_sum[key] else 0.0
        return (
            f"| **{label}** | {fmt.format(b_sum[key])} | {fmt.format(a_sum[key])} | "
            f"{'+' if diff > 0 else ''}{diff:g}" + (f" ({'+' if pct > 0 else ''}{pct:.1f}%)" if b_sum[key] else "") + " |"
        )

    def _s_cell(o: Optional[Dict[str, Any]]) -> str:
        return ("yes" if (o or {}).get("skill_triggered", True) else "no") if (o or {}).get("skill_trigger_relevant") else "n/a"

    cols = [
        "Case", "Before Status", "After Status",
        *(["Before .tf Read", "After .tf Read"] if show_tf else []),
        *(["Before Skill Read", "After Skill Read"] if show_trig else []),
        "Before Tokens", "After Tokens", "Token Delta (%)", "Before Tools", "After Tools", "Before Time (s)", "After Time (s)",
    ]
    lines = [
        "## Cluster Toolkit Agent Evaluation: Before vs. After Impact Report", "", "### 1. Executive Summary Scorecard", "",
        "| Metric | Before | After | Delta |", "| :--- | :--- | :--- | :--- |",
        f"| **Pass Rate** | {b_sum['passed_cases']}/{b_sum['total_cases']} ({b_sum['pass_rate']:.1f}%) | "
        f"{a_sum['passed_cases']}/{a_sum['total_cases']} ({a_sum['pass_rate']:.1f}%) | {'+' if rate_diff > 0 else ''}{rate_diff:.1f} pp |",
        *[_exec_row(label, key, fmt) for label, key, fmt, _ in _active_scorecard_metrics(show_tf, show_re, show_trig)],
        "", "### 2. Per-Case Comparison Table", "", "| " + " | ".join(cols) + " |", "| " + " | ".join(":---" for _ in cols) + " |",
    ]
    for cname in list(dict.fromkeys(list(a_cases.keys()) + list(b_cases.keys()))):
        bc, ac = b_cases.get(cname, {}), a_cases.get(cname, {})
        bo = bc.get("oracles") if isinstance(bc.get("oracles"), dict) else bc
        ao = ac.get("oracles") if isinstance(ac.get("oracles"), dict) else ac
        b_st = "PASS" if bc.get("passed") else ("FAIL" if bc else "N/A")
        a_st = "PASS" if ac.get("passed") else ("FAIL" if ac else "N/A")
        b_tok, a_tok = int((bo or {}).get("total_tokens", 0) or 0), int((ao or {}).get("total_tokens", 0) or 0)
        pct = ((a_tok - b_tok) / b_tok) * 100.0 if b_tok > 0 else 0.0
        row = [
            f"`{sanitize_markdown_cell(cname)}`", b_st, a_st,
            *([str(_tf_count(bo)), str(_tf_count(ao))] if show_tf else []),
            *([_s_cell(bo), _s_cell(ao)] if show_trig else []),
            f"{b_tok:,}", f"{a_tok:,}", f"{'+' if pct > 0 else ''}{pct:.1f}%" if b_tok > 0 else "N/A",
            str(int((bo or {}).get("tool_calls_total", 0) or 0)), str(int((ao or {}).get("tool_calls_total", 0) or 0)),
            f"{float(bc.get('latency_seconds', 0.0) or 0.0):.1f}s", f"{float(ac.get('latency_seconds', 0.0) or 0.0):.1f}s",
        ]
        lines.append("| " + " | ".join(row) + " |")
    reg_ok, regressions = check_baseline_regression(before_data, after_data)
    lines.extend([
        "",
        "### 3. Baseline Regression Check",
        "",
        "No pass-to-fail regressions detected." if reg_ok else "\n".join(f"- `[REGRESSION]` {r}" for r in regressions),
    ])
    return "\n".join(lines) + "\n"
