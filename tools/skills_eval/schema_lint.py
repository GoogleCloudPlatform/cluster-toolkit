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

"""Schema, frontmatter, and command-safety linting for Cluster Toolkit agent skills."""

from dataclasses import dataclass, field
import os
from pathlib import Path
import re
import sys
from typing import Any, Dict, FrozenSet, List, NoReturn, Optional, Set, Tuple
import yaml

REPO_ROOT = str(Path(__file__).resolve().parents[2])
NAME_REGEX = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")
FRONTMATTER_REGEX = re.compile(r"^\ufeff?[ \t]*---[ \t]*\r?\n(.*?)\r?\n[ \t]*---[ \t]*(?:\r?\n(.*))?$", re.DOTALL)
VALID_STATUSES: Tuple[str, ...] = ("experimental", "stable")
REQUIRED_FRONTMATTER: Tuple[str, ...] = ("name", "description")

FORBIDDEN_MUTATING_PATTERNS = [re.compile(p, re.IGNORECASE) for p in (
    r"(?:^|[\s,;`|&\"'()\[\]/\\])(?:rm|rmdir|shred|wipefs|fdisk|gdisk|parted|mkfs|mkswap|scancel|sbatch|kill|pkill|killall|shutdown|reboot|poweroff|init\s+0|umount)\b",
    r"(?:^|[\s,;`|&\"'()\[\]/\\])dd\s+.*?\bof=",
    r">\s*(?:/dev/|/etc/)",
    r"\bkubectl\b[^;&|\n]*?\b(?:delete|drain|cordon|patch|replace|scale|apply|create|edit|run|taint|label)\b",
    r"\bscontrol\b[^;&|\n]*?\b(?:update|delete|reboot|drain)\b",
    r"\bterraform\b[^;&|\n]*?\b(?:destroy|apply|taint|import)\b",
    r"\bhelm\b[^;&|\n]*?\b(?:uninstall|delete)\b",
    r"\bgcloud\b[^;&|\n]*?\b(?:delete|destroy|terminate|purge)\b",
    r"\b(?:gcluster|ghpc)\b[^;&|\n]*?\b(?:deploy|destroy|submit|cancel|config(?:\s+set)?)\b",
)]
ASSERTION_LIST_FIELDS = ("forbidden_commands", "expect_keywords_all", "expect_keywords_any", "script_validators", "expectations")
VALID_SUITE_KEYS = frozenset({"suite_name", "description", "expectations", "forbidden_commands", "extra_scorers", "disabled_scorers", "cases"})
VALID_CASE_KEYS = frozenset({
    "name", "description", "prompt", "expect_blocked_action", "live_setup", "target_blueprint", "smoke",
    "extra_scorers", "disabled_scorers", "ignore_suite_expectations", "ignore_suite_forbidden_commands", *ASSERTION_LIST_FIELDS,
})
BLUEPRINT_HYGIENE_SCORERS: Tuple[str, ...] = ("no_tf_read", "no_generated_file_edits", "ai_assisted_tag", "blueprint_reexpand")
DEFAULT_SCORERS: Tuple[str, ...] = ("safety_killswitch", "no_tracked_template_edits", "skill_triggering", "script_validators", "expectations")
SCORER_REGISTRY: FrozenSet[str] = frozenset(DEFAULT_SCORERS + ("blueprint_hygiene", "no_workspace_writes") + BLUEPRINT_HYGIENE_SCORERS)
TF_PATH_REGEX = re.compile(r"""(?:^|[\s"'=/])((?:community/)?modules/[a-zA-Z0-9_./-]+\.tf)\b""")
MAX_LIVE_TOOL_CALLS = 60
_COERCIBLE_BOOL_STRS = frozenset({"true", "false", "1", "0", "yes", "no"})


@dataclass(frozen=True)
class LintResult:
    skill_name: str
    passed: bool
    message: str


@dataclass(frozen=True)
class TestCaseResult:
    case_name: str
    passed: bool
    message: str
    latency_seconds: float = 0.0


@dataclass(frozen=True)
class EvalResult:
    skill_name: str
    passed: bool
    message: str
    cases: List[TestCaseResult] = field(default_factory=list)


def _die(msg: str) -> NoReturn:
    sys.stderr.write(f"Error: {msg}\n")
    sys.exit(2)


def _write_file(path: str, content: str) -> None:
    if os.path.dirname(path):
        os.makedirs(os.path.dirname(path), exist_ok=True)
    Path(path).write_text(content, encoding="utf-8")


def parse_frontmatter(content: str) -> Tuple[Dict[str, Any], str]:
    """Parse YAML frontmatter and markdown body supporting CRLF, BOM, and comments."""
    clean = content.lstrip("\ufeff")
    match = FRONTMATTER_REGEX.match(clean)
    if not match:
        parts = clean.split("---", 2) if (clean.startswith("---") and "\n---" in clean) else []
        if len(parts) >= 3 and parts[1].strip() == "":
            return {}, parts[2].lstrip("\r\n")
        raise ValueError("Missing or malformed YAML frontmatter enclosed in '---'")
    try:
        parsed = yaml.safe_load(match.group(1))
    except yaml.YAMLError as e:
        raise ValueError(f"YAML parsing error: {e}") from e
    if parsed is not None and not isinstance(parsed, dict):
        raise ValueError(f"Frontmatter must be a YAML dictionary, got {type(parsed).__name__}")
    return parsed or {}, match.group(2) or ""


def check_command_safety(command_str: str) -> Tuple[bool, str]:
    """Check if a command contains prohibited mutating patterns using word boundaries."""
    cmd = command_str.strip()
    if any(p.search(cmd) for p in FORBIDDEN_MUTATING_PATTERNS):
        return False, f"Forbidden mutating command/primitive detected in '{cmd}'. Only read-only diagnostic commands are permitted."
    return True, "Safe"


def _coerce_to_string_list(val: Any) -> List[str]:
    """Coerce None, strings, scalars, or lists into a sanitized list of non-empty strings."""
    if isinstance(val, (str, int, float, bool)):
        return [s.strip() for s in str(val).splitlines() if s.strip()]
    return [str(x).strip() for x in val if x is not None and str(x).strip()] if isinstance(val, list) else []


def _expand_scorers(names: Any) -> Set[str]:
    raw = set(_coerce_to_string_list(names))
    return (raw - {"blueprint_hygiene"}) | (set(BLUEPRINT_HYGIENE_SCORERS) if "blueprint_hygiene" in raw else set())


def resolve_active_scorers(suite_data: Optional[Dict[str, Any]] = None, case: Optional[Dict[str, Any]] = None) -> Set[str]:
    """Resolve active scorers using 4-step suite-then-case precedence with non-disableable safety_killswitch."""
    s, c = suite_data or {}, case or {}
    active = (
        (set(DEFAULT_SCORERS) | _expand_scorers(s.get("extra_scorers")))
        - _expand_scorers(s.get("disabled_scorers"))
        | _expand_scorers(c.get("extra_scorers"))
    ) - _expand_scorers(c.get("disabled_scorers"))
    return active | {"safety_killswitch"}


resolve_case_scorers = resolve_active_scorers


def resolve_case_expectations(suite_data: Optional[Dict[str, Any]] = None, case: Optional[Dict[str, Any]] = None) -> List[str]:
    """Append suite-level expectations after case expectations unless blocked or opted out."""
    s, c = suite_data or {}, case or {}
    case_exp, b = _coerce_to_string_list(c.get("expectations")), c.get("expect_blocked_action")
    if c.get("ignore_suite_expectations") is True or (b.strip().lower() in ("true", "1", "yes") if isinstance(b, str) else bool(b)):
        return case_exp
    return case_exp + [e for e in _coerce_to_string_list(s.get("expectations")) if e not in case_exp]


def resolve_case_forbidden_commands(suite_data: Optional[Dict[str, Any]] = None, case: Optional[Dict[str, Any]] = None) -> List[str]:
    """Append deduplicated suite-level forbidden_commands after case forbidden_commands unless opted out."""
    s, c = suite_data or {}, case or {}
    case_fc = _coerce_to_string_list(c.get("forbidden_commands"))
    if c.get("ignore_suite_forbidden_commands") is True:
        return case_fc
    return list(dict.fromkeys([*case_fc, *_coerce_to_string_list(s.get("forbidden_commands"))]))


def _is_valid_blocked_flag(val: Any) -> bool:
    if val is None or isinstance(val, bool):
        return True
    if type(val) is int:
        return val in (0, 1)
    if isinstance(val, str):
        return val.strip().lower() in _COERCIBLE_BOOL_STRS
    return False


def normalize_case(raw_case: Dict[str, Any], index: int = 1, suite_data: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    """Normalize a test case dictionary to ensure safe, uniform types across all runners."""
    case, cname, prompt = dict(raw_case), raw_case.get("name"), raw_case.get("prompt")
    case["name"] = cname.strip() if isinstance(cname, str) and cname.strip() else f"case_{index}"
    case["prompt"] = str(prompt).strip() if prompt is not None else ""
    for list_field in (*ASSERTION_LIST_FIELDS, "extra_scorers", "disabled_scorers"):
        case[list_field] = _coerce_to_string_list(case.get(list_field))
    blocked, target_bp = case.get("expect_blocked_action"), case.get("target_blueprint")
    case["expect_blocked_action"] = blocked.strip().lower() in ("true", "1", "yes") if isinstance(blocked, str) else bool(blocked)
    case["ignore_suite_expectations"] = bool(case.get("ignore_suite_expectations", False))
    case["ignore_suite_forbidden_commands"] = bool(case.get("ignore_suite_forbidden_commands", False))
    case["expectations"] = resolve_case_expectations(suite_data, case)
    case["forbidden_commands"] = resolve_case_forbidden_commands(suite_data, case)
    if suite_data is not None or "extra_scorers" in raw_case or "disabled_scorers" in raw_case:
        case["_active_scorers"] = sorted(resolve_active_scorers(suite_data, raw_case))
    case["target_blueprint"] = target_bp.strip() if isinstance(target_bp, str) and target_bp.strip() else None
    case["smoke"] = bool(case.get("smoke", False))
    return case


def filter_cases(raw_cases: List[Dict[str, Any]], cases_arg: Optional[str], skill_name: str = "", allow_empty: bool = False) -> List[Dict[str, Any]]:
    """Filter EVAL.yaml cases by comma-separated boundary-aware prefix tokens (or 'all' / 'smoke' / '<skill>/<case>')."""
    tokens = [t.strip() for t in (cases_arg or "").split(",") if t.strip()]
    if not tokens or any(t.lower() == "all" for t in tokens):
        return list(raw_cases)
    indexed = [(i, normalize_case(rc, i + 1)["name"] if isinstance(rc, dict) else f"case_{i + 1}", rc) for i, rc in enumerate(raw_cases)]
    selected: List[int] = []
    for raw_tok in tokens:
        tok_skill, tok = (raw_tok.rsplit("/", 1) if "/" in raw_tok else ("", raw_tok))
        if tok_skill and skill_name and os.path.basename(tok_skill.rstrip("/")) != skill_name:
            matched: List[int] = []
        elif tok.lower() == "all":
            matched = [i for i, _, _ in indexed]
        elif tok.lower() == "smoke":
            matched = [i for i, _, rc in indexed if isinstance(rc, dict) and rc.get("smoke") is True]
            if not matched:
                pos = [i for i, _, rc in indexed if isinstance(rc, dict) and not normalize_case(rc, i + 1)["expect_blocked_action"]]
                neg = [i for i, _, rc in indexed if isinstance(rc, dict) and normalize_case(rc, i + 1)["expect_blocked_action"]]
                matched = (pos[:1] + neg[:1]) if (pos and neg) else [i for i, _, _ in indexed[:2]]
        else:
            matched = [i for i, cname, _ in indexed if cname == tok or cname.startswith(tok + "_")]
        if not matched and not allow_empty:
            raise ValueError(f"--cases filter token '{raw_tok}' matched 0 cases. Available cases: {', '.join(c for _, c, _ in indexed)}")
        selected.extend(i for i in matched if i not in selected)
    return [rc for i, _, rc in indexed if i in set(selected)]


def lint_eval_yaml(skill_path: str, eval_file: Optional[str] = None) -> Tuple[bool, str]:
    """Validate EVAL.yaml presence, parseability, and essential invariants."""
    eval_path, label = eval_file or os.path.join(skill_path, "EVAL.yaml"), eval_file or f"EVAL.yaml in {skill_path}"
    if not os.path.isfile(eval_path):
        return False, f"Missing {label}. Every skill must include an EVAL.yaml test suite with test cases."
    try:
        data = yaml.safe_load(Path(eval_path).read_text(encoding="utf-8"))
    except yaml.YAMLError as e:
        return False, f"EVAL.yaml parse error in {eval_file or skill_path}: {e}"
    if not isinstance(data, dict) or not isinstance(data.get("cases"), list) or not data["cases"]:
        return False, f"{label} must contain a non-empty 'cases' list."
    unknown_s = sorted(set(data) - VALID_SUITE_KEYS)
    if unknown_s:
        return False, f"EVAL.yaml suite in {skill_path} contains unknown field(s): {', '.join(unknown_s)}."

    def _check_list(owner: str, fname: str, val: Any, is_scorer: bool = False) -> Optional[str]:
        if isinstance(val, dict):
            return f"EVAL.yaml {owner} in {skill_path} field '{fname}' cannot be a dictionary/mapping."
        bad = next((x for x in val if isinstance(x, (dict, list))), None) if isinstance(val, list) else None
        if bad is not None:
            return f"EVAL.yaml {owner} in {skill_path} field '{fname}' contains invalid nested structure {type(bad).__name__}."
        bad_sc = sorted(set(_coerce_to_string_list(val)) - SCORER_REGISTRY) if is_scorer else []
        return f"EVAL.yaml {owner} in {skill_path} field '{fname}' contains unknown scorer(s): {', '.join(bad_sc)}." if bad_sc else None

    suite_err = next(filter(None, (
        _check_list("suite", fn, data.get(fn), sc)
        for fn, sc in (("expectations", False), ("forbidden_commands", False), ("extra_scorers", True), ("disabled_scorers", True))
    )), None)
    if suite_err:
        return False, suite_err
    for idx, raw_case in enumerate(data["cases"]):
        if not isinstance(raw_case, dict):
            return False, f"EVAL.yaml case #{idx + 1} in {skill_path} must be a dictionary, got {type(raw_case).__name__}."
        cname = raw_case.get("name") or f"case_{idx + 1}"
        prompt, tbp, unknown = raw_case.get("prompt"), raw_case.get("target_blueprint"), sorted(set(raw_case) - VALID_CASE_KEYS)
        bad_c = next((msg for cond, msg in (
            (bool(unknown), f"EVAL.yaml case '{cname}' in {skill_path} contains unknown field(s): {', '.join(unknown)}."),
            (not prompt or not isinstance(prompt, str) or not prompt.strip(), f"EVAL.yaml case '{cname}' in {skill_path} is missing a non-empty 'prompt'."),
            (tbp is not None and (not isinstance(tbp, str) or not tbp.strip()), f"EVAL.yaml case '{cname}' in {skill_path} field 'target_blueprint' must be a non-empty string."),
            (not _is_valid_blocked_flag(raw_case.get("expect_blocked_action")), f"EVAL.yaml case '{cname}' in {skill_path} field 'expect_blocked_action' must be a boolean."),
            *(
                (raw_case.get(bf) is not None and not isinstance(raw_case.get(bf), bool), f"EVAL.yaml case '{cname}' in {skill_path} field '{bf}' must be a boolean.")
                for bf in ("smoke", "ignore_suite_expectations", "ignore_suite_forbidden_commands")
            ),
        ) if cond), None) or next(filter(None, (
            _check_list(f"case '{cname}'", fn, raw_case.get(fn), sc)
            for fn, sc in (*((f, False) for f in ASSERTION_LIST_FIELDS), ("extra_scorers", True), ("disabled_scorers", True))
        )), None)
        if bad_c:
            return False, bad_c
        if not (
            raw_case.get("expect_blocked_action") is not None
            or resolve_case_expectations(data, raw_case)
            or resolve_case_forbidden_commands(data, raw_case)
            or any(raw_case.get(f) for f in ASSERTION_LIST_FIELDS)
        ):
            return False, (
                f"EVAL.yaml case '{cname}' in {skill_path} must specify at least one assertion field "
                "(valid fields: 'expect_keywords_all', 'expect_keywords_any', 'forbidden_commands', 'expect_blocked_action', 'script_validators', 'expectations')."
            )
    return True, "Valid EVAL.yaml schema"


def lint_skill(skill_path: str, eval_file: Optional[str] = None) -> LintResult:
    """Validate skill against Cluster Toolkit specifications and invariants."""
    skill_name, skill_md_path = os.path.basename(os.path.normpath(skill_path)), os.path.join(skill_path, "SKILL.md")
    if not os.path.isfile(skill_md_path):
        return LintResult(skill_name, False, f"Missing SKILL.md in {skill_path}. Create {skill_name}/SKILL.md with valid YAML frontmatter on Line 1.")
    try:
        meta, body = parse_frontmatter(Path(skill_md_path).read_text(encoding="utf-8"))
    except Exception as e:  # noqa: BLE001
        return LintResult(skill_name, False, f"SKILL.md frontmatter parse error in {skill_path}: {e}")
    for req in REQUIRED_FRONTMATTER:
        if req not in meta or meta[req] is None or not str(meta[req]).strip():
            return LintResult(skill_name, False, f"Missing or empty required frontmatter key '{req}' in {skill_name}/SKILL.md.")
    name, desc, compat = str(meta["name"]).strip(), str(meta["description"]).strip(), meta.get("compatibility")
    status = meta.get("status") or (meta["metadata"].get("status", "stable") if isinstance(meta.get("metadata"), dict) else "stable")
    raw_ro, raw_tools = meta.get("allowed_read_only_commands"), meta.get("allowed-tools")
    raw_tools_list = [str(t) for t in raw_tools] if isinstance(raw_tools, list) else (re.findall(r"\w+\([^)]*\)|\S+", raw_tools) if isinstance(raw_tools, str) else [])
    cmds = ([str(c) for c in raw_ro] if isinstance(raw_ro, list) else ([raw_ro] if isinstance(raw_ro, str) else [])) + [
        f"Bash({sub})" if (t.startswith("Bash(") and t.endswith(")")) else t
        for t in raw_tools_list
        for sub in (re.split(r",\s*", t[5:-1]) if (t.startswith("Bash(") and t.endswith(")")) else [t])
    ]
    bad_msg = next((msg for cond, msg in (
        (not (1 <= len(name) <= 64), f"Skill name length must be between 1 and 64 characters (got {len(name)})."),
        (not NAME_REGEX.match(name), f"Skill name '{name}' violates naming specification. Use lowercase alphanumeric characters and single hyphens only (e.g. 'my-skill-name')."),
        (name != skill_name, f"Frontmatter name '{name}' does not match directory '{skill_name}'. Update 'name: {skill_name}' in SKILL.md or rename directory to '{name}'."),
        (not (1 <= len(desc) <= 1024), f"Description length must be between 1 and 1024 characters (got {len(desc)})."),
        (len(desc) > 300, f"Description exceeds Cluster Toolkit limit ({len(desc)}/300 chars). Shorten by {len(desc) - 300} characters to keep Tier 1 routing tokens compact."),
        (
            "compatibility" in meta and not (isinstance(compat, dict) or (isinstance(compat, str) and 1 <= len(compat) <= 500)),
            f"Compatibility field must be 1-500 characters (got {len(compat)})." if isinstance(compat, str) else "Compatibility field must be a string or mapping.",
        ),
        (status not in VALID_STATUSES, f"Invalid status '{status}' (must be 'stable' or 'experimental')."),
        (
            status == "experimental" and "[!WARNING]" not in (body or ""),
            f"Experimental skill '{skill_name}' must include an upfront '[!WARNING]' markdown callout in body (e.g. '> [!WARNING]\\n> This skill is experimental...').",
        ),
    ) if cond), None) or next((msg for ok, msg in (check_command_safety(c) for c in cmds) if not ok), None)
    if bad_msg:
        return LintResult(skill_name, False, bad_msg)
    eval_ok, eval_msg = lint_eval_yaml(skill_path, eval_file=eval_file)
    return LintResult(skill_name, eval_ok, "Valid frontmatter and EVAL.yaml schema" if eval_ok else eval_msg)
