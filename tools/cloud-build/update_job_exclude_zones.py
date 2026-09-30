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

"""Safely extracts or updates EXCLUDE_ZONES in Kubernetes Job manifests.

Uses only the Python 3 standard library so it executes reliably in minimal
environments (such as gcr.io/cloud-builders/gcloud) without external dependencies.
"""

import argparse
import os
import re
import sys


def update_job_yaml(content: str, zones_to_inject: str = None) -> tuple:
    """Extracts or updates EXCLUDE_ZONES in the target test runner container.

    Args:
        content: The text content of the job.yaml file.
        zones_to_inject: Space-separated zones to inject. If None, only extracts.

    Returns:
        tuple of (extracted_zones: str, modified_content: str)
    """
    lines = content.splitlines(keepends=True)

    # 1. Locate the containers block
    containers_line = -1
    for i, line in enumerate(lines):
        if re.match(r"^[ ]*containers:\s*(?:#.*)?$", line):
            containers_line = i
            break

    if containers_line == -1:
        return "", content

    # 2. Locate each container and its line range
    item_indent = None
    container_ranges = []
    current_start = None
    current_name = None

    for i in range(containers_line + 1, len(lines)):
        line = lines[i]
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        indent = len(line) - len(line.lstrip(" "))

        m_item = re.match(
            r'^([ ]*)-\s*(?:name:\s*["\']?([^\s#"\'\n]+)["\']?)?', line
        )
        if item_indent is None and m_item:
            item_indent = len(m_item.group(1))

        if (
            item_indent is not None
            and indent == item_indent
            and line[item_indent : item_indent + 2] == "- "
        ):
            if current_start is not None:
                container_ranges.append((current_start, i, current_name))
            current_start = i
            current_name = m_item.group(2) if m_item else None
        elif item_indent is not None and (
            indent < item_indent
            or (
                indent == item_indent
                and line[item_indent : item_indent + 2] != "- "
            )
        ):
            if current_start is not None:
                container_ranges.append((current_start, i, current_name))
                current_start = None
            break

    if current_start is not None:
        container_ranges.append((current_start, len(lines), current_name))

    if not container_ranges:
        return "", content

    # Scan for container names if not inline with '- '
    final_containers = []
    for c_start, c_end, c_name in container_ranges:
        if not c_name:
            for j in range(c_start, min(c_start + 10, c_end)):
                if lines[j].strip().startswith("#"):
                    continue
                nm = re.match(
                    r'^[ ]*name:\s*["\']?([^\s#"\'\n]+)["\']?', lines[j]
                )
                if nm:
                    c_name = nm.group(1)
                    break
        final_containers.append((c_start, c_end, c_name))

    # Target container: prefer 'runner', else first container
    target_start, target_end, target_name = final_containers[0]
    for c_start, c_end, c_name in final_containers:
        if c_name == "runner":
            target_start, target_end, target_name = c_start, c_end, c_name
            break

    c_lines = lines[target_start:target_end]

    # 3. Search for EXCLUDE_ZONES inside target container
    extracted_val = ""
    value_line_idx = -1
    env_line_idx = -1
    env_empty_idx = -1
    env_indent = None
    first_env_item_idx = -1
    first_env_item_indent = None

    for rel_i, line in enumerate(c_lines):
        abs_i = target_start + rel_i
        stripped = line.strip()
        if stripped.startswith("#"):
            continue

        m_env = re.match(r"^([ ]*)env:\s*(?:#.*)?$", line)
        if m_env and env_line_idx == -1:
            env_line_idx = abs_i
            env_indent = m_env.group(1)
            continue

        m_env_empty = re.match(r"^([ ]*)env:\s*\[\s*\]\s*(?:#.*)?$", line)
        if m_env_empty and env_line_idx == -1:
            env_empty_idx = abs_i
            env_indent = m_env_empty.group(1)
            continue

        if env_line_idx != -1 and first_env_item_idx == -1:
            m_it = re.match(r"^([ ]*)-\s*", line)
            if m_it:
                first_env_item_idx = abs_i
                first_env_item_indent = m_it.group(1)

        if re.search(r'name:\s*["\']?EXCLUDE_ZONES["\']?\s*(?:#.*)?$', line):
            # Check backwards (if value: preceded name:)
            for b in range(abs_i - 1, max(target_start, abs_i - 3), -1):
                if lines[b].strip().startswith("#"):
                    continue
                if re.match(r"^[ ]*-\s*", lines[b + 1]):
                    break
                vm = re.search(r'value:\s*["\']?([^"\'#\n]*)["\']?', lines[b])
                if vm:
                    value_line_idx = b
                    extracted_val = vm.group(1).strip()
                    break
            # Check forwards (if value: succeeded name:)
            if value_line_idx == -1:
                for f in range(abs_i + 1, min(target_end, abs_i + 3)):
                    if lines[f].strip().startswith("#"):
                        continue
                    if re.match(r"^[ ]*-\s*", lines[f]):
                        break
                    vm = re.search(
                        r'value:\s*["\']?([^"\'#\n]*)["\']?', lines[f]
                    )
                    if vm:
                        value_line_idx = f
                        extracted_val = vm.group(1).strip()
                        break

    if zones_to_inject is None:
        return extracted_val, content

    # 4. Inject or Update
    if value_line_idx != -1:
        old_line = lines[value_line_idx]
        prefix_match = re.match(r"^([ ]*(?:-\s*)?)value:", old_line)
        prefix = (
            prefix_match.group(1)
            if prefix_match
            else re.match(r"^([ ]*)", old_line).group(1)
        )
        lines[value_line_idx] = f'{prefix}value: "{zones_to_inject}"\n'
    elif env_line_idx != -1:
        if first_env_item_indent is not None:
            it_ind = first_env_item_indent
            val_ind = it_ind + "  "
        else:
            it_ind = env_indent
            val_ind = it_ind + "  "
        injection = f'{it_ind}- name: EXCLUDE_ZONES\n{val_ind}value: "{zones_to_inject}"\n'
        lines.insert(env_line_idx + 1, injection)
    elif env_empty_idx != -1:
        # Replace empty flow list 'env: []' with block list
        lines[env_empty_idx] = (
            f'{env_indent}env:\n'
            f'{env_indent}- name: EXCLUDE_ZONES\n'
            f'{env_indent}  value: "{zones_to_inject}"\n'
        )
    else:
        img_line_idx = -1
        img_ind = None
        for rel_i, line in enumerate(c_lines):
            if lines[target_start + rel_i].strip().startswith("#"):
                continue
            m = re.match(r"^([ ]*)image:", line)
            if m:
                img_line_idx = target_start + rel_i
                img_ind = m.group(1)
                break
        if img_line_idx != -1:
            injection = f'{img_ind}env:\n{img_ind}- name: EXCLUDE_ZONES\n{img_ind}  value: "{zones_to_inject}"\n'
            lines.insert(img_line_idx + 1, injection)
        else:
            c_base_ind = " " * (
                item_indent + 2 if item_indent is not None else 8
            )
            injection = f'{c_base_ind}env:\n{c_base_ind}- name: EXCLUDE_ZONES\n{c_base_ind}  value: "{zones_to_inject}"\n'
            lines.insert(target_end, injection)

    return extracted_val, "".join(lines)


def main():
    parser = argparse.ArgumentParser(
        description="Update or extract EXCLUDE_ZONES in a Kubernetes Job manifest."
    )
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument(
        "--extract", action="store_true", help="Extract existing EXCLUDE_ZONES"
    )
    group.add_argument(
        "--inject", action="store_true", help="Inject or update EXCLUDE_ZONES"
    )
    parser.add_argument(
        "--file",
        required=True,
        help="Path to job.yaml",
    )
    parser.add_argument(
        "--zones",
        default="",
        help="Space-separated zone list to inject",
    )
    args = parser.parse_args()

    if not os.path.isfile(args.file):
        print(f"File not found: {args.file}", file=sys.stderr)
        sys.exit(1)

    with open(args.file, "r", encoding="utf-8") as f:
        content = f.read()

    if args.extract:
        val, _ = update_job_yaml(content, None)
        if val:
            print(val)
    elif args.inject:
        _, modified = update_job_yaml(content, args.zones)
        tmp_file = f"{args.file}.tmp"
        with open(tmp_file, "w", encoding="utf-8") as f:
            f.write(modified)
        os.replace(tmp_file, args.file)


if __name__ == "__main__":
    main()
