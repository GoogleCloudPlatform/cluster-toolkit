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

"""Validates module README YAML snippets, relative Markdown links, and anchors."""

import glob
import os
import re
import subprocess
import sys
import urllib.parse
import yaml

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
TF_DOCS_MARKER = "<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->"
REMOTE_SOURCE_PREFIXES = (
    "github.com/",
    "git::",
    "git@",
    "gcs::",
    "http://",
    "https://",
    "registry.terraform.io/",
)


def _get_non_ignored_repo_files() -> set[str] | None:
    """Returns absolute paths of all tracked and non-gitignored files in REPO_ROOT, or None if git is unavailable."""
    try:
        res = subprocess.run(
            ["git", "-C", REPO_ROOT, "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="surrogateescape",
            check=True,
        )
        return {
            os.path.normpath(os.path.join(REPO_ROOT, rel))
            for rel in res.stdout.split("\0")
            if rel
        }
    except Exception:
        return None


def _get_non_ignored_repo_dirs(non_ignored: set[str] | None) -> set[str] | None:
    """Returns absolute paths of all directories containing non-ignored repo files."""
    if non_ignored is None:
        return None
    dirs: set[str] = {REPO_ROOT}
    for fpath in non_ignored:
        cur = os.path.dirname(fpath)
        while cur and cur not in dirs and cur.startswith(REPO_ROOT):
            dirs.add(cur)
            parent = os.path.dirname(cur)
            if parent == cur:
                break
            cur = parent
    return dirs


def _is_checked_repo_file(abs_path: str, non_ignored: set[str] | None) -> bool:
    """Returns True if `abs_path` is an existing file that is not excluded by .gitignore."""
    norm = os.path.normpath(abs_path)
    if not os.path.isfile(norm) or "node_modules" in norm:
        return False
    if non_ignored is not None and norm not in non_ignored:
        return False
    return True


def _is_checked_repo_path(
    abs_path: str,
    non_ignored: set[str] | None,
    non_ignored_dirs: set[str] | None = None,
) -> bool:
    """Returns True if `abs_path` is a non-ignored repo file or a directory containing non-ignored repo files."""
    norm = os.path.normpath(abs_path)
    if not os.path.exists(norm) or "node_modules" in norm:
        return False
    if os.path.isfile(norm):
        return _is_checked_repo_file(norm, non_ignored)
    if os.path.isdir(norm):
        if non_ignored is None:
            return True
        if non_ignored_dirs is None:
            non_ignored_dirs = _get_non_ignored_repo_dirs(non_ignored)
        return non_ignored_dirs is None or norm in non_ignored_dirs
    return False


def _find_markdown_files_under(
    rel_folders: tuple[str, ...], non_ignored: set[str] | None
) -> list[str]:
    """Returns sorted absolute paths of non-ignored .md files under `rel_folders` without walking gitignored trees."""
    if non_ignored is not None:
        prefixes = tuple(
            os.path.normpath(os.path.join(REPO_ROOT, folder)) + os.sep for folder in rel_folders
        )
        return sorted(
            p
            for p in non_ignored
            if p.endswith(".md") and p.startswith(prefixes) and _is_checked_repo_file(p, non_ignored)
        )
    matched: set[str] = set()
    for folder in rel_folders:
        matched.update(
            glob.glob(
                os.path.join(REPO_ROOT, os.path.normpath(folder), "**", "*.md"), recursive=True
            )
        )
    return sorted(p for p in matched if _is_checked_repo_file(p, non_ignored))


def _hcl_block_body(content: str, start: int) -> str:
    """Returns the body of the HCL block whose opening brace ends at index `start`."""
    depth, i, n = 1, start, len(content)
    while depth and i < n:
        ch = content[i]
        if ch == "#":
            i = content.find("\n", i + 1)
            if i == -1:
                break
        elif ch == "/" and i + 1 < n and content[i + 1] == "/":
            i = content.find("\n", i + 2)
            if i == -1:
                break
        elif ch == "/" and i + 1 < n and content[i + 1] == "*":
            end_comment = content.find("*/", i + 2)
            if end_comment == -1:
                break
            i = end_comment + 1
        elif ch == '"':
            i += 1
            while i < n:
                if content[i] == "\\":
                    i += 2
                    continue
                if content[i] == '"':
                    break
                i += 1
        elif ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
        i += 1
    return content[start:i]


def _strip_hcl_comments(content: str) -> str:
    """Strips HCL comments (#, //, /* ... */) outside double-quoted strings."""
    out: list[str] = []
    i, n = 0, len(content)
    while i < n:
        ch = content[i]
        if ch == "#" or (ch == "/" and i + 1 < n and content[i + 1] == "/"):
            nl = content.find("\n", i + 1)
            if nl == -1:
                break
            out.append("\n")
            i = nl + 1
        elif ch == "/" and i + 1 < n and content[i + 1] == "*":
            end_comment = content.find("*/", i + 2)
            if end_comment == -1:
                break
            out.append(" ")
            i = end_comment + 2
        elif ch == '"':
            out.append(ch)
            i += 1
            while i < n:
                out.append(content[i])
                if content[i] == "\\":
                    if i + 1 < n:
                        out.append(content[i + 1])
                    i += 2
                    continue
                if content[i] == '"':
                    i += 1
                    break
                i += 1
        else:
            out.append(ch)
            i += 1
    return "".join(out)


def get_tf_vars_and_outputs(
    mod_dir: str, non_ignored: set[str] | None = None
) -> tuple[set[str], set[str], set[str]]:
    """Extracts active variables, deprecated variables, and outputs from a module directory.

    In Cluster Toolkit modules, deprecated tombstone variables are kept in variables.tf with a
    `validation { condition = var.<name> == null }` (or `length(var.<name>) == 0`) block.
    """
    inputs: set[str] = set()
    deprecated_inputs: set[str] = set()
    outputs: set[str] = set()
    patterns = [
        os.path.join(mod_dir, "*.tf"),
        os.path.join(mod_dir, "*.pkr.hcl"),
    ]
    for pattern in patterns:
        for tf_file in glob.glob(pattern):
            if not _is_checked_repo_file(tf_file, non_ignored):
                continue
            try:
                with open(tf_file, encoding="utf-8") as f:
                    content = _strip_hcl_comments(f.read())
            except OSError:
                continue
            for m in re.finditer(r'variable\s+"([^"]+)"\s*\{', content):
                name = re.escape(m.group(1))
                body = _hcl_block_body(content, m.end())
                if re.search(
                    rf"condition\s*=\s*(?:var\.{name}\s*==\s*null|length\(var\.{name}\)\s*==\s*0)\s*$",
                    body,
                    re.MULTILINE,
                ):
                    deprecated_inputs.add(m.group(1))
                else:
                    inputs.add(m.group(1))
            for m in re.finditer(r'output\s+"([^"]+)"', content):
                outputs.add(m.group(1))
    return inputs, deprecated_inputs, outputs


def is_remote_source(src: str) -> bool:
    """Returns True if module source refers to a remote Git or Terraform Registry URL."""
    return src.startswith(REMOTE_SOURCE_PREFIXES)


def extract_modules_from_yaml(data: object) -> list[dict]:
    """Extracts module dictionaries from parsed YAML data."""
    mods: list[dict] = []
    if isinstance(data, list):
        for item in data:
            if isinstance(item, dict):
                if "source" in item and "id" in item:
                    mods.append(item)
                if isinstance(item.get("modules"), list):
                    for sub in item["modules"]:
                        if isinstance(sub, dict) and "source" in sub and "id" in sub:
                            mods.append(sub)
    elif isinstance(data, dict):
        if "source" in data and "id" in data:
            mods.append(data)
        for grp in data.get("deployment_groups") or []:
            if isinstance(grp, dict):
                for item in grp.get("modules") or []:
                    if isinstance(item, dict) and "source" in item and "id" in item:
                        mods.append(item)
        if isinstance(data.get("modules"), list):
            for item in data["modules"]:
                if isinstance(item, dict) and "source" in item and "id" in item:
                    mods.append(item)
    return mods


def validate_yaml_block(
    block: str,
    rel_path: str,
    tf_cache: dict[str, tuple[set[str], set[str], set[str]]],
    require_defined_refs: bool = False,
    non_ignored: set[str] | None = None,
) -> list[str]:
    """Validates a YAML block containing Cluster Toolkit module definitions."""
    errors: list[str] = []
    lines = block.splitlines()
    if lines and all(
        not line.strip() or line.lstrip().startswith(">") for line in lines
    ):
        block = "\n".join(re.sub(r"^\s*>\s?", "", line) for line in lines)
    try:
        docs = list(yaml.safe_load_all(block))
    except yaml.YAMLError as exc:
        first_line = str(exc).splitlines()[0]
        errors.append(f"{rel_path}: invalid YAML syntax in snippet: {first_line}")
        return errors

    mods: list[dict] = []
    for doc in docs:
        mods.extend(extract_modules_from_yaml(doc))

    mod_by_id: dict[str, str] = {}
    for mod in mods:
        mid = str(mod.get("id", ""))
        raw_src = str(mod.get("source", "")).strip()
        if not raw_src or is_remote_source(raw_src):
            continue
        norm_src = raw_src.lstrip("./")
        abs_src = os.path.join(REPO_ROOT, norm_src)
        hcl_files = [
            f
            for f in (
                glob.glob(os.path.join(abs_src, "*.tf"))
                + glob.glob(os.path.join(abs_src, "*.pkr.hcl"))
            )
            if _is_checked_repo_file(f, non_ignored)
        ]
        has_hcl = os.path.isdir(abs_src) and bool(hcl_files)
        if not has_hcl:
            errors.append(
                f"{rel_path}: module '{mid}' references non-existent source '{raw_src}'"
            )
            continue
        mod_by_id[mid] = norm_src

        if norm_src not in tf_cache:
            tf_cache[norm_src] = get_tf_vars_and_outputs(abs_src, non_ignored)
        inputs, deprecated_inputs, mod_outputs = tf_cache[norm_src]

        settings = mod.get("settings")
        if isinstance(settings, dict):
            for key in settings:
                if key in deprecated_inputs:
                    errors.append(
                        f"{rel_path}: module '{mid}' ({norm_src}) setting '{key}' "
                        "is deprecated in variables.tf"
                    )
                elif key not in inputs:
                    errors.append(
                        f"{rel_path}: module '{mid}' ({norm_src}) setting '{key}' "
                        "does not exist in variables.tf"
                    )

        declared_outputs = mod.get("outputs")
        if isinstance(declared_outputs, list):
            for out_item in declared_outputs:
                out_name = (
                    out_item.get("name")
                    if isinstance(out_item, dict)
                    else out_item
                )
                if isinstance(out_name, str) and out_name not in mod_outputs:
                    errors.append(
                        f"{rel_path}: module '{mid}' ({norm_src}) output '{out_name}' "
                        "does not exist in outputs.tf"
                    )

    for ref in re.finditer(
        r"(?:\$\(([a-zA-Z0-9_-]+)\.([a-zA-Z0-9_-]+)\)|\(\(module\.([a-zA-Z0-9_-]+)\.([a-zA-Z0-9_-]+))",
        block,
    ):
        ref_id = ref.group(1) or ref.group(3)
        ref_out = ref.group(2) or ref.group(4)
        if ref_id == "vars":
            continue
        ref_src = mod_by_id.get(ref_id)
        if require_defined_refs and not ref_src:
            errors.append(
                f"{rel_path}: references undeclared module '{ref_id}.{ref_out}'"
            )
            continue
        if ref_src and ref_src in tf_cache:
            _, _, ref_outputs = tf_cache[ref_src]
            if ref_out not in ref_outputs:
                errors.append(
                    f"{rel_path}: snippet references non-existent output "
                    f"'{ref_id}.{ref_out}' ({ref_src})"
                )

    for mod in mods:
        mid = str(mod.get("id", ""))
        raw_src = str(mod.get("source", "")).strip()
        if not raw_src or is_remote_source(raw_src):
            continue
        norm_src = raw_src.lstrip("./")
        if norm_src not in tf_cache:
            continue
        inputs, _, _ = tf_cache[norm_src]

        use_list = mod.get("use")
        if not use_list:
            continue
        if not isinstance(use_list, list):
            errors.append(f"{rel_path}: module '{mid}' has non-list 'use' field")
            continue

        for used_id in use_list:
            if not isinstance(used_id, str):
                errors.append(
                    f"{rel_path}: module '{mid}' has invalid non-string entry in 'use': {used_id}"
                )
                continue
            used_src = mod_by_id.get(used_id)
            if require_defined_refs and not used_src:
                errors.append(
                    f"{rel_path}: module '{mid}' lists undeclared module '{used_id}' in 'use'"
                )
                continue
            if not used_src or used_src not in tf_cache:
                continue
            _, _, used_outputs = tf_cache[used_src]
            shared = (inputs & used_outputs) - {"labels"}
            if not shared:
                errors.append(
                    f"{rel_path}: module '{mid}' ({norm_src}) lists '{used_id}' "
                    f"({used_src}) in 'use', but no output from '{used_id}' matches an input of '{mid}'"
                )

    return errors


def validate_module_readme_snippets(non_ignored: set[str] | None = None) -> list[str]:
    """Validates YAML snippets in module/blueprint READMEs, docs/**/*.md, and os_compatibility_tests."""
    errors: list[str] = []
    if non_ignored is None:
        non_ignored = _get_non_ignored_repo_files()
    examples_root_dir = os.path.join(REPO_ROOT, "examples")
    readme_paths = sorted(
        {
            p
            for p in (
                glob.glob(os.path.join(REPO_ROOT, "modules", "*", "*", "README.md"))
                + glob.glob(
                    os.path.join(REPO_ROOT, "community", "modules", "*", "*", "README.md")
                )
            )
            if _is_checked_repo_file(p, non_ignored)
        }
        | {
            p
            for p in _find_markdown_files_under(
                ("docs", "examples", "community/examples", "community/front-end"),
                non_ignored,
            )
            if os.path.dirname(p) != examples_root_dir
        }
    )
    tf_cache: dict[str, tuple[set[str], set[str], set[str]]] = {}

    for abs_path in readme_paths:
        rel_path = os.path.relpath(abs_path, REPO_ROOT)
        try:
            with open(abs_path, encoding="utf-8") as f:
                text = f.read()
        except OSError as exc:
            errors.append(f"{rel_path}: failed to read file: {exc}")
            continue

        pre_hook = text.split(TF_DOCS_MARKER)[0]
        fence_lines = [
            line for line in pre_hook.splitlines() if line.strip().startswith("```")
        ]
        if len(fence_lines) % 2 != 0:
            errors.append(f"{rel_path}: unclosed Markdown code fence (odd number of ``` markers)")
            continue

        for match in re.finditer(r"```([^\n]*)\n(.*?)```", pre_hook, re.DOTALL):
            lang = match.group(1).strip().lower()
            block = match.group(2)
            if lang not in ("yaml", "yml", ""):
                continue
            if "source:" not in block or "id:" not in block:
                continue
            errors.extend(
                validate_yaml_block(
                    block, rel_path, tf_cache, require_defined_refs=False, non_ignored=non_ignored
                )
            )

    os_compat_blueprints = [
        p
        for p in sorted(
            glob.glob(
                os.path.join(REPO_ROOT, "tools", "validate_configs", "os_compatibility_tests", "*.yaml")
            )
        )
        if _is_checked_repo_file(p, non_ignored)
    ]
    for abs_path in os_compat_blueprints:
        rel_path = os.path.relpath(abs_path, REPO_ROOT)
        try:
            with open(abs_path, encoding="utf-8") as f:
                text = f.read()
        except OSError as exc:
            errors.append(f"{rel_path}: failed to read file: {exc}")
            continue
        errors.extend(
            validate_yaml_block(
                text, rel_path, tf_cache, require_defined_refs=True, non_ignored=non_ignored
            )
        )

    return errors


def slugify_github(heading: str) -> str:
    """Converts a Markdown heading string into its GitHub anchor slug."""
    h = re.sub(r"\s+#+$", "", heading.strip())
    h = re.sub(r"!\[[^\]]*\](?:\([^\)]*\)|\[[^\]]*\])?", "", h)
    h = re.sub(r"\[([^\]]*)\](?:\([^\)]*\)|\[[^\]]*\])?", r"\1", h)
    h = h.replace("\\_", "_").lower()
    res: list[str] = []
    for ch in h:
        if ch.isalnum() or ch in ("-", "_"):
            res.append(ch)
        elif ch == " ":
            res.append("-")
    return "".join(res)


def get_markdown_anchors(abs_md_path: str) -> set[str]:
    """Returns all valid HTML and heading anchors in a Markdown file."""
    anchors: set[str] = set()
    try:
        with open(abs_md_path, encoding="utf-8") as f:
            text = f.read()
    except OSError:
        return anchors

    for m in re.finditer(r'<a\s+(?:name|id)=["\']?([^"\'>\s]+)["\']?', text, re.IGNORECASE):
        anchors.add(m.group(1).replace("\\_", "_").lower())

    counts: dict[str, int] = {}
    in_fence = False
    for line in text.splitlines():
        if line.strip().startswith("```"):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        m = re.match(r"^#{1,6}\s+(.*)$", line)
        if m:
            slug = slugify_github(m.group(1))
            if slug in counts:
                counts[slug] += 1
                anchors.add(f"{slug}-{counts[slug]}")
            else:
                counts[slug] = 0
                anchors.add(slug)
    return anchors


def validate_markdown_links_and_cli(non_ignored: set[str] | None = None) -> list[str]:
    """Validates relative links, heading anchors, Cloud Shell repo paths, and legacy ./ghpc usage."""
    errors: list[str] = []
    if non_ignored is None:
        non_ignored = _get_non_ignored_repo_files()
    non_ignored_dirs = _get_non_ignored_repo_dirs(non_ignored)
    md_files = [
        p
        for p in (
            os.path.join(REPO_ROOT, root_doc)
            for root_doc in ("README.md", "AGENTS.md", "CONTRIBUTING.md")
        )
        if _is_checked_repo_file(p, non_ignored)
    ] + _find_markdown_files_under(
        ("docs", "examples", "modules", "community", "pkg", "cmd", "tools", "skills"),
        non_ignored,
    )
    anchor_cache: dict[str, set[str]] = {}

    for abs_path in md_files:
        rel_path = os.path.relpath(abs_path, REPO_ROOT)
        try:
            with open(abs_path, encoding="utf-8") as f:
                text = f.read()
        except OSError as exc:
            errors.append(f"{rel_path}: failed to read file: {exc}")
            continue

        # Check for legacy ./ghpc CLI invocations
        if re.search(r"(?:^|\s)\./ghpc\b", text):
            errors.append(f"{rel_path}: contains legacy './ghpc' CLI reference (use './gcluster')")

        no_fences = re.sub(r"```.*?```", "", text, flags=re.DOTALL)
        no_code = re.sub(r"`[^`\n]+`", "", no_fences)

        ref_defs: set[str] = set()
        targets: list[str] = []
        for m in re.finditer(r"\[(?:!\[[^\]]*\]\([^\)]*\)|[^\]]+)\]\(([^\)\s]+)(?:\s+\"[^\"]*\")?\)", no_code):
            targets.append(m.group(1))
        for m in re.finditer(r"^\s*\[([^\]]+)\]:\s*(\S+)", no_code, flags=re.MULTILINE):
            ref_defs.add(re.sub(r"\s+", " ", m.group(1).strip().lower()))
            targets.append(m.group(2))
        for m in re.finditer(r"(?<![!\w\])])\[([^\][\n]+)\]\[([^\][\n]*)\]", no_code):
            ref_label = re.sub(r"\s+", " ", (m.group(2) or m.group(1)).strip().lower())
            if ref_label not in ref_defs:
                errors.append(
                    f"{rel_path}: undefined reference-style link '[{m.group(1)}][{m.group(2)}]'"
                )
        for m in re.finditer(r'<a\s+[^>]*href=["\']([^"\'>\s]+)["\']', no_code, flags=re.IGNORECASE):
            targets.append(m.group(1))

        for target in targets:
            if target.startswith(("http://", "https://")):
                if "shell.cloud.google.com/cloudshell/editor" in target:
                    parsed = urllib.parse.urlparse(target)
                    qs = urllib.parse.parse_qs(parsed.query)
                    repo_url = qs.get("cloudshell_git_repo", [""])[0]
                    if "hpc-toolkit" in repo_url or "cluster-toolkit" in repo_url:
                        for param in ("cloudshell_open_in_editor", "cloudshell_tutorial"):
                            for rel_target in qs.get(param, []):
                                abs_target = os.path.join(REPO_ROOT, rel_target.lstrip("/"))
                                if not _is_checked_repo_path(
                                    abs_target, non_ignored, non_ignored_dirs
                                ):
                                    errors.append(
                                        f"{rel_path}: Cloud Shell link {param}='{rel_target}' "
                                        "points to non-existent file"
                                    )
                continue

            if target.startswith(("mailto:", "<")):
                continue

            if "#" in target:
                file_part, anchor_part = target.split("#", 1)
            else:
                file_part, anchor_part = target, ""

            file_part = file_part.replace("\\_", "_")
            anchor_part = anchor_part.replace("\\_", "_")

            if not file_part:
                abs_target = abs_path
            elif file_part.startswith("/"):
                abs_target = os.path.join(REPO_ROOT, file_part.lstrip("/"))
            else:
                abs_target = os.path.normpath(os.path.join(os.path.dirname(abs_path), file_part))

            if os.path.commonpath(
                [REPO_ROOT, os.path.abspath(abs_target)]
            ) != REPO_ROOT or not _is_checked_repo_path(
                abs_target, non_ignored, non_ignored_dirs
            ):
                errors.append(f"{rel_path}: broken relative file link '{target}'")
                continue

            if anchor_part and os.path.isfile(abs_target) and abs_target.endswith(".md"):
                if abs_target not in anchor_cache:
                    anchor_cache[abs_target] = get_markdown_anchors(abs_target)
                anchors = anchor_cache[abs_target]
                norm_anchor = anchor_part.lower()
                if norm_anchor not in anchors:
                    target_rel = os.path.relpath(abs_target, REPO_ROOT)
                    errors.append(
                        f"{rel_path}: broken heading anchor '#{anchor_part}' in '{target_rel}'"
                    )

    return errors


def main() -> int:
    non_ignored = _get_non_ignored_repo_files()
    snippet_errors = validate_module_readme_snippets(non_ignored)
    link_errors = validate_markdown_links_and_cli(non_ignored)
    all_errors = snippet_errors + link_errors

    if all_errors:
        print(f"Found {len(all_errors)} documentation validation error(s):", file=sys.stderr)
        for err in all_errors:
            print(f"  - {err}", file=sys.stderr)
        return 1

    print("All module README YAML snippets, relative links, and anchors are valid.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
