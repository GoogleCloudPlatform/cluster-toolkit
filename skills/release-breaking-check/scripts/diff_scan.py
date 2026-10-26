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

"""Deterministic release-breaking signal scanner for Cluster Toolkit diffs.

Reads a unified diff (from git or a file) and emits JSON describing mechanical
signals that *may* make an existing deployment fail, be replaced, or behave
differently on the next `gcluster deploy`. It never decides on its own; it tells
a human or model where to look. Standard library only; no network access.

Usage:
  diff_scan.py --base origin/develop --head HEAD
  diff_scan.py --diff-file pr.diff
"""

import argparse
import json
import re
import subprocess
import sys
from typing import Dict, List, Optional, Tuple

SEVERITY_THRESHOLDS = (("high", 6), ("medium", 3), ("low", 1))

# Terraform attributes whose change usually forces resource replacement.
FORCENEW_ATTRS = (
    "name", "image", "source_image", "instance_image", "machine_type", "zone",
    "region", "disk_type", "disk_size_gb", "boot_disk", "network", "subnetwork",
    "network_name", "subnetwork_self_link", "address", "prefix_length",
    "release_channel", "enable_confidential_compute", "threads_per_core",
    "guest_accelerator", "accelerator_type", "min_cpu_platform",
)

# Example blueprint settings that replace node pools / VMs / storage when changed
# or that override a module default when newly added. Matched as substrings so
# `system_node_pool_machine_type` matches `machine_type`.
EXAMPLE_IMPACT_KEYS = (
    "machine_type", "instance_image", "image_family", "zone", "region",
    "disk_type", "disk_size_gb", "release_channel", "version_prefix",
    "gpu_driver", "guest_accelerator", "threads_per_core", "enable_dranet",
    "reservation", "static_node_count", "total_min_nodes", "total_max_nodes",
    "_version", "size_gb", "tier", "capacity_gib", "local_mount", "network_storage",
    "ip_range", "subnetwork", "cidr", "port", "enable_slurm_auth", "helm_release_name",
    "accelerator", "node_count", "spot", "local_ssd", "family", "public_ips", "enable_",
)


def _impact_key(key: str) -> bool:
    return any(sub in key for sub in EXAMPLE_IMPACT_KEYS)

VERSION_PIN_RE = re.compile(
    r"(kueue|jobset|slurm|cuda|driver|nccl|htcondor|lustre|gke)[a-z_]*?"
    r"(version|image|family|ref|tag)\b", re.IGNORECASE)

EXAMPLE_PATH_RE = re.compile(r"^(community/)?examples/.+\.ya?ml$")
MODULE_PATH_RE = re.compile(r"^(community/)?modules/([^/]+/[^/]+)/")
SCRIPT_PATH_RE = re.compile(
    r"^(community/)?modules/.+/(scripts|files|templates)/|\.(tftpl|tpl)$")
GCLUSTER_CORE_RE = re.compile(r"^(pkg/(config|validators|modulewriter|modulereader)|cmd)/")


class Hunk:
    """One file's worth of diff lines."""

    def __init__(self, old_path: str, new_path: str):
        self.old_path = old_path
        self.new_path = new_path
        self.removed: List[str] = []
        self.added: List[str] = []
        self.deleted_file = False
        self.new_file = False
        self.renamed = old_path != new_path and old_path != "/dev/null" and new_path != "/dev/null"

    @property
    def path(self) -> str:
        return self.new_path if self.new_path != "/dev/null" else self.old_path


def parse_unified_diff(text: str) -> List[Hunk]:
    hunks: List[Hunk] = []
    cur: Optional[Hunk] = None
    old_path = new_path = ""
    for line in text.splitlines():
        if line.startswith("diff --git "):
            if cur:
                hunks.append(cur)
            m = re.match(r"diff --git a/(.*?) b/(.*)$", line)
            old_path, new_path = (m.group(1), m.group(2)) if m else ("", "")
            cur = Hunk(old_path, new_path)
            continue
        if cur is None:
            continue
        if line.startswith("deleted file mode"):
            cur.deleted_file = True
        elif line.startswith("new file mode"):
            cur.new_file = True
        elif line.startswith("--- ") or line.startswith("+++ "):
            continue
        elif line.startswith("-"):
            cur.removed.append(line[1:])
        elif line.startswith("+"):
            cur.added.append(line[1:])
    if cur:
        hunks.append(cur)
    return hunks


def _signal(sid: str, path: str, detail: str, weight: int) -> Dict:
    return {"id": sid, "file": path, "detail": detail, "weight": weight}


def _tf_blocks(lines: List[str], kind: str) -> Dict[str, str]:
    """Map block label -> full header for `variable "x"`, `output "x"`, `resource "t" "n"`."""
    out = {}
    pat = re.compile(r'^\s*%s\s+"([^"]+)"(?:\s+"([^"]+)")?\s*\{' % kind)
    for ln in lines:
        m = pat.match(ln)
        if m:
            label = m.group(1) + ("." + m.group(2) if m.group(2) else "")
            out[label] = ln.strip()
    return out


def _yaml_keys(lines: List[str]) -> Dict[str, str]:
    """Very small YAML line reader: `key: value` -> value (ignores nesting)."""
    out = {}
    for ln in lines:
        m = re.match(r"^\s*([A-Za-z_][\w-]*):\s*(.*?)\s*$", ln)
        if m:
            out[m.group(1)] = m.group(2)
    return out


def scan_terraform(h: Hunk) -> List[Dict]:
    sigs: List[Dict] = []
    p = h.path
    base = p.rsplit("/", 1)[-1]

    if base == "variables.tf":
        old_vars, new_vars = _tf_blocks(h.removed, "variable"), _tf_blocks(h.added, "variable")
        for v in old_vars:
            if v not in new_vars and not any(f'variable "{v}"' in ln for ln in h.added):
                sigs.append(_signal("tf_variable_removed", p, v, 4))
        for ln in h.removed:
            if re.match(r"^\s*default\s*=", ln):
                sigs.append(_signal("tf_variable_default_changed", p, ln.strip()[:80], 2))
                break
        if any(re.match(r"^\s*(validation\s*\{|condition\s*=)", ln) for ln in h.added):
            sigs.append(_signal("tf_validation_tightened", p, "validation block added/changed", 2))

    elif base == "outputs.tf":
        old_out, new_out = _tf_blocks(h.removed, "output"), _tf_blocks(h.added, "output")
        for o in old_out:
            if o not in new_out:
                sigs.append(_signal("tf_output_removed", p, o, 3))

    elif p.endswith(".tf"):
        old_res, new_res = _tf_blocks(h.removed, "resource"), _tf_blocks(h.added, "resource")
        moved = any(re.match(r"^\s*moved\s*\{", ln) for ln in h.added)
        for r in old_res:
            if r not in new_res and not moved:
                sigs.append(_signal("tf_resource_removed_or_renamed", p, r, 4))
        old_mod, new_mod = _tf_blocks(h.removed, "module"), _tf_blocks(h.added, "module")
        for m in old_mod:
            if m not in new_mod and not moved:
                sigs.append(_signal("tf_module_removed_or_renamed", p, m, 3))
        for ln in h.removed:
            m = re.match(r"^\s*([a-z_]+)\s*=\s*(.+)$", ln)
            if m and m.group(1) in FORCENEW_ATTRS:
                sigs.append(_signal("tf_forcenew_literal_changed", p, f"{m.group(1)}: {m.group(2).strip()[:60]}", 3))
        for ln in h.removed + h.added:
            if re.search(r'\b(release_name|helm_release_name|name)\s*=\s*"[^"]*"', ln) and "helm" in p:
                sigs.append(_signal("helm_release_name_changed", p, ln.strip()[:80], 3))
                break
        for ln in h.removed:
            if VERSION_PIN_RE.search(ln) and "=" in ln:
                sigs.append(_signal("version_pin_changed", p, ln.strip()[:80], 2))
                break
    return sigs


def scan_example(h: Hunk) -> List[Dict]:
    sigs: List[Dict] = []
    p = h.path
    if h.deleted_file:
        return [_signal("example_deleted", p, "example blueprint removed", 4)]
    if h.renamed:
        sigs.append(_signal("example_renamed", p, f"from {h.old_path}", 4))
    if h.new_file:
        return sigs  # additive

    old_k, new_k = _yaml_keys(h.removed), _yaml_keys(h.added)

    # vars: removed / renamed (line-level approximation: a top-level-looking key gone)
    for k in old_k:
        if k not in new_k and re.match(r"^[a-z][a-z0-9_]*$", k) and k not in ("id", "source", "use", "settings", "kind"):
            sigs.append(_signal("example_var_or_setting_removed", p, k, 3))
            break

    # module id / source changed
    for k in ("id", "source"):
        if k in old_k and k in new_k and old_k[k] != new_k[k]:
            sigs.append(_signal("example_module_id_or_source_changed", p, f"{k}: {old_k[k]} -> {new_k[k]}", 5))
    if "use" in old_k or any(ln.strip().startswith("- ") and "use" in "".join(h.removed[:3]) for ln in h.removed):
        if any(re.match(r"^\s*use:\s*", ln) for ln in h.removed + h.added):
            sigs.append(_signal("example_use_changed", p, "use: list changed", 2))

    # impactful settings changed in place, or newly added (overrides a module default)
    for k in old_k:
        if k in new_k and old_k[k] != new_k[k] and k not in ("id", "source") and _impact_key(k):
            sigs.append(_signal("example_setting_changed", p, f"{k}: {old_k[k]} -> {new_k[k]}", 3))
    for k in new_k:
        if k not in old_k and _impact_key(k) and new_k[k] != "":
            sigs.append(_signal("example_setting_added", p, f"{k}: {new_k[k]}", 2))

    # new var without default (heuristic: added `  foo:` under vars with empty value)
    for ln in h.added:
        m = re.match(r"^\s{2}([a-z][a-z0-9_]*):\s*$", ln)
        if m and m.group(1) not in ("vars", "settings", "use", "outputs", "deployment_groups", "modules", "terraform_backend_defaults"):
            sigs.append(_signal("example_var_added_required", p, m.group(1), 3))
            break
    return sigs


def scan_other(h: Hunk) -> List[Dict]:
    sigs: List[Dict] = []
    p = h.path
    if SCRIPT_PATH_RE.search(p) and (h.removed or h.added) and not h.new_file:
        sigs.append(_signal("rendered_script_changed", p, "script/template content changed (metadata or GCS object replacement)", 2))
    if GCLUSTER_CORE_RE.match(p):
        if any(re.search(r"(errors?\.New|fmt\.Errorf|HintError|ValidationError|return.*err)", ln) for ln in h.added):
            sigs.append(_signal("gcluster_validation_changed", p, "new or changed error path in gcluster core", 2))
    return sigs


def scan(diff_text: str) -> Dict:
    hunks = parse_unified_diff(diff_text)
    signals: List[Dict] = []
    touched_examples: List[str] = []
    touched_modules: List[str] = []
    for h in hunks:
        p = h.path
        if EXAMPLE_PATH_RE.match(p):
            touched_examples.append(p)
            signals.extend(scan_example(h))
        elif p.endswith(".tf"):
            m = MODULE_PATH_RE.match(p)
            if m and m.group(2) not in touched_modules:
                touched_modules.append(m.group(2))
            signals.extend(scan_terraform(h))
        signals.extend(scan_other(h))

    # de-duplicate identical signals
    seen: set = set()
    uniq: List[Dict] = []
    for s in signals:
        key = (s["id"], s["file"], s["detail"])
        if key not in seen:
            seen.add(key)
            uniq.append(s)

    total = sum(s["weight"] for s in uniq)
    severity = "none"
    for label, threshold in SEVERITY_THRESHOLDS:
        if total >= threshold:
            severity = label
            break
    return {
        "signals": uniq,
        "signal_count": len(uniq),
        "touched_examples": sorted(set(touched_examples)),
        "touched_modules": sorted(set(touched_modules)),
        "severity_hint": severity,
        "weight_total": total,
    }


def git_diff(base: str, head: str) -> str:
    return subprocess.run(
        ["git", "diff", "--no-color", "--no-ext-diff", f"{base}...{head}"],
        capture_output=True, text=True, check=True).stdout


def main(argv: Optional[List[str]] = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    src = ap.add_mutually_exclusive_group(required=True)
    src.add_argument("--diff-file", help="path to a unified diff ('-' for stdin)")
    src.add_argument("--base", help="base git ref (use with --head)")
    ap.add_argument("--head", default="HEAD", help="head git ref (default HEAD)")
    ap.add_argument("--pretty", action="store_true", help="indent JSON output")
    args = ap.parse_args(argv)

    if args.diff_file:
        text = sys.stdin.read() if args.diff_file == "-" else open(args.diff_file, encoding="utf-8", errors="replace").read()
    else:
        text = git_diff(args.base, args.head)
    result = scan(text)
    print(json.dumps(result, indent=2 if args.pretty else None))
    return 0


if __name__ == "__main__":
    sys.exit(main())
