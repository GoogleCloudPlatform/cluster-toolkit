---
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

name: cluster-blueprint-assistant
description: >
  Create, customize, validate, and debug Cluster Toolkit Slurm/GKE blueprints
  and modules with gcluster: inspect modules/ or community/modules/ inputs,
  outputs, use: wiring, and GCP APIs; tune nodes, machine types, zones,
  Spot/reservations, Lustre/Filestore; or fix expand/create/deploy errors.
compatibility: "Requires the gcluster binary (built from this repo via `make`) and a Cluster Toolkit checkout."
metadata:
  author: GoogleCloudPlatform
  status: experimental
  domain: blueprints
allowed-tools: Bash(make) Bash(./gcluster explain:*) Bash(./gcluster expand:*) Bash(./gcluster create:*) Bash(gcluster explain:*) Bash(gcluster expand:*) Bash(gcluster create:*)
allowed_read_only_commands:
  - "make"
  - "./gcluster explain"
  - "./gcluster expand"
  - "./gcluster create"
  - "gcluster explain"
  - "gcluster expand"
  - "gcluster create"
---

# Cluster Toolkit Blueprint Assistant

> [!WARNING]
> This skill is experimental. Read-only inspection and validation commands (`./gcluster explain`, `./gcluster expand`, `./gcluster create`) may run directly; always halt with a `[PROPOSED REMEDIATION PLAN]` and wait for explicit user confirmation before deploying, destroying, or deleting files or cloud resources.

> **Scope:** These rules apply to Cluster Toolkit user and operator workflows (including inspecting module inputs, outputs, `use:` wiring, and required GCP APIs under `modules/` or `community/modules/`); repository maintainers explicitly authoring or refactoring Cluster Toolkit Go source (`cmd/`, `pkg/`, `tools/`) or editing module `.tf` implementation code when explicitly requested may read and edit `.tf` and Go source files normally.

## Core rules (read before touching any YAML)

These rules apply even if the user asks otherwise. If a request conflicts with them, refuse that step, explain why, and offer the `./gcluster`-based alternative.

1. **Start from `./gcluster explain` or the catalog, never from scratch, and never read `.tf` files.** Run
   `./gcluster explain <specs...>` (e.g. `./gcluster explain slurm a3u lustre`) or
   read [`skills/cluster-blueprint-assistant/references/catalog.yaml`](references/catalog.yaml) (and [`llms.txt`](../../llms.txt) /
   [`llms-full.txt`](../../llms-full.txt)) to find an existing
   blueprint under `examples/` or `community/examples/` that already matches the
   request. **Never open, read, `cat`, or `grep` any `.tf` file (`variables.tf`,
   `outputs.tf`, `main.tf`), and never run recursive directory searches (`grep -r`,
   `rg`, `find`, or shell/Python directory walks) across `modules/` or
   `community/modules/`** (even to look up `metadata.yaml` or module outputs, because
   recursive scans open `.tf` files). Do not run `ls`, `find`, or `grep` across
   `examples/` or `.tf` files to discover templates or modules; run `./gcluster explain <module>` (e.g.
   `./gcluster explain modules/file-system/managed-lustre`) or
   `./gcluster explain <module> -f yaml` (which prints full optional input types,
   default values, and descriptions), or check the catalog's
   `module_wiring` section for a module's `required_inputs`, which module sources
   satisfy them (`recommended_use`), and which modules consume its outputs
   (`consumed_by`). Use `./gcluster expand <blueprint.yaml> --vars project_id=<project>`
   to trigger built-in metadata and graph validators. If the request is a question
   ("do we have…", "can I get…",
   "what's in…"), answer it using `./gcluster explain` / `catalog.yaml` and
   confirm the approach with the user **before** editing anything.
2. **Customize through a new, untracked `deployment.yaml`, not module blocks.** If the request
   only changes project, region/zone, node counts, machine types,
   Spot/reservation, or storage size:
   - **Pattern 1 — All requested knobs already exist in `vars:`:** Create or edit
     **only** a **new, untracked** `deployment.yaml` (or `<name>-deployment.yaml`)
     passed with `-d`, containing `ai_assisted: true` and `vars:` (omit `terraform_backend_defaults` (`type: gcs`, `configuration.bucket`)
     unless the user provides a GCS state bucket). Do **not** copy or edit the
     blueprint `.yaml` file, and **never modify git-tracked companion `*-deployment.yaml`
     files** under `examples/` or `community/examples/` in place (even if the example's README tells human users to edit it; read them only as a reference for variable names).
   - **Pattern 2 — Promoting a hardcoded or module-default setting to `vars:`:** When a
     requested setting is hardcoded inside a module block (or uses a module default) and is missing from
     `vars:`, never modify a tracked template under `examples/` or
     `community/examples/` in place. Instead, copy the template to a new file in
     the **same directory** with `ai_assisted: true` (or reuse an existing custom
     copy in place), add the missing knob to `vars:` with a default **equal to
     the previously hardcoded value or module default**, reference it in the module via
     `$(vars.<name>)`, and put the user's override value in `deployment.yaml`.
   - **A template only exposes knobs it already set explicitly.** Tracked
     blueprints promote to `vars:` only settings that were literal values in a
     module block; they do not add Spot, reservation, DWS or any other input the
     module block was silent about. If the knob you need is not in `vars:`
     (check the catalog entry), it is Pattern 2 — copy the blueprint. Never add the
     knob to the tracked template, and never pass it as a bare module-input name
     in `deployment.yaml` hoping it will be picked up.
   - **Mutually exclusive consumption models (Spot vs. DWS Flex Start vs. Reservations):**
     Spot VMs (`spot` / `enable_spot_vm`), DWS Flex Start (`enable_flex_start` / `dws_flex`),
     and Reservations (`reservation` / `SPECIFIC_RESERVATION`) cannot be combined on the
     same node pool or nodeset. If the user asks to enable Spot on a DWS Flex Start or
     Reservation blueprint (or vice versa), **never silently disable** the blueprint's
     existing consumption model (e.g. do not set `enable_flex_start: false` unprompted);
     stop, explain that the two models are mutually exclusive, and ask the user which
     single consumption model they want. If a GKE blueprint hardcodes `consume_reservation_type: SPECIFIC_RESERVATION` (e.g., `gke-tpu-v5e.yaml`) and the user gave no reservation name, never set `reservation: ""`; stop and ask for a reservation name or whether they want a custom copy (`ai_assisted: true`) for on-demand, Spot, or DWS Flex Start.
   - On casual or vague requests (e.g. a "small" or "cheap" cluster with Spot VMs
     and N nodes), override **only** the requested Spot and node-count variables
     (such as `compute_enable_spot_vm: true`, `compute_node_count_dynamic_max: 2` —
     do not also modify unrequested partitions like `debug` or `h3`) in
     `deployment.yaml`—keep the blueprint's default machine types (never downgrade
     `c2-standard-60` to `e2-standard-2`, which fails `gvnic_enabled` validation,
     unless the user explicitly requests that machine type)
     and never remove partitions or storage modules unprompted.
   - **Hardware-locked blueprints (`# Hardware-locked:` on `machine_type`):**
     When customizing a hardware-locked blueprint using variables already exposed
     in its `vars:` block, follow Pattern 1: write an untracked `deployment.yaml`
     (`ai_assisted: true`) and never copy or edit the hardware-locked blueprint
     template. If the user asks for a different accelerator or machine series (e.g., `a3-highgpu-8g` on `gke-a4.yaml`), never copy and swap `machine_type` — switch to the matching blueprint via `./gcluster explain`, or stop and inform the user if no blueprint matches both the target hardware and specialized features (inference gateway, DWS Flex Start, GCSFuse + Local SSD).
   - Do **not** add, remove, rename or restructure `deployment_groups[].modules[]`
     entries for value overrides.
3. **Only edit module blocks when adding or removing capability** (Pattern 3 — e.g.
   adding a Managed Lustre instance or a second node pool) — and do it on a
   **copy**, not the shared template: copy the blueprint to a new file in the
   **same directory** (e.g. `examples/gke-a4/my-gke-a4.yaml`) or copy the whole
   folder, because blueprints reference sibling files via
   `$(ghpc_stage("./…"))`. Never modify tracked template files under
   `examples/` or `community/examples/` in place (once a custom copy
   exists—identifiable by `ai_assisted: true` or untracked `git status`—edit
   that copy directly in place for follow-ups rather than copying it a second
   time). Copy the module block from `./gcluster explain <module>` (or
   `./gcluster explain <module> -f yaml` for optional input types, defaults, and
   descriptions) or a blueprint in the catalog that already has it, keep its `use:`
   list, and check `module_wiring.<source>.required_inputs` (or run
   `./gcluster explain <module>`) to confirm every required input is
   satisfied by a module in `use:`. **Never open, read, `cat`, or `grep` any `.tf`
   file (`variables.tf`, `outputs.tf`, `main.tf`), and never run recursive searches
   (`grep -r`, `rg`, `find`, or shell/Python directory walks) across `modules/` or
   `community/modules/`**—run `./gcluster expand <blueprint.yaml> --vars project_id=<project>`
   to validate all module inputs and `use:` wiring.
4. **Mark agent-authored blueprints and deployment files.** Add `ai_assisted: true` at the top level
   of any new untracked `deployment.yaml` and any blueprint or deployment `.yaml` you create, copy, edit, or fix (if an existing custom
   file you are updating lacks `ai_assisted: true`, add it). If `./gcluster` reports `field ai_assisted not found in type config.DeploymentSettings`, the binary is older than this field: run `make` and retry instead of removing `ai_assisted`.
5. **Validate with gcluster, fix the blueprint, never the output.**
   `./gcluster expand` and `./gcluster create` are the validation step. Once
   `./gcluster create` or `./gcluster expand` succeeds, validation is complete—**do
   not run `terraform` afterwards**; if the user also asked to deploy, proceed
   directly to the confirmation gate in rule 6. If
   `./gcluster expand`, `./gcluster create`, or `./gcluster deploy` fails, fix
   the blueprint YAML or `deployment.yaml` and re-run `./gcluster`. Never bypass validators via CLI flags (`-l IGNORE`,
   `--validation-level IGNORE`, `--skip-validators`), environment variables
   (`GHPC_MOCK_MACHINE_CONFIG`), or YAML fields (`validation_level: IGNORE`, or
   `skip: true` under `validators:`), **even if the user explicitly asks to skip a
   validator by name**—refuse the bypass, explain that skipping
   `test_machine_type_in_zone` or `test_zone_in_region` still fails during
   module expansion (404 Compute API error for Slurm `machine_configs` lookup)
   or deployment (404 or region/zone mismatch error) when the zone or machine
   type is invalid for the selected region/zone, and ask
   the user for a supported region, zone, or machine type. **Sole exception:** the
   offline command in "3. Validate" (`GHPC_MOCK_MACHINE_CONFIG` +
   `--skip-validators`), used only when there are no GCP credentials or
   `project_id` is a placeholder—never to get past a validator that failed
   against a real project. Never edit
   generated files under `<deployment_name>/` (`main.tf`, `variables.tf`,
   `terraform.tfvars`, `defaults.auto.pkrvars.hcl`, `.ghpc/`) and never run
   `terraform` in any form — not `init`, `validate`, `plan`, `apply`, `destroy`,
   or `fmt` — even if the user asks you to; refuse, explain that `./gcluster` owns
   validation and deployment, and if the user is asking to apply or destroy a
   deployment (e.g. `terraform init && terraform apply` in `<deployment_name>/primary`),
   immediately present a `[PROPOSED REMEDIATION PLAN]` (Rule 6) gating
   `./gcluster deploy <deployment_name>` (or `./gcluster destroy <deployment_name>`)
   and wait for explicit `yes`—never put `terraform` commands inside the remediation
   plan. `./gcluster` has no dry-run `plan` command: if asked for a
   dry-run plan or resource preview before deploying, never run `terraform plan` or
   pipe `yes n | ./gcluster deploy`; instead run `./gcluster explain <blueprint.yaml>`
   and inspect `<deployment_name>/.ghpc/artifacts/expanded_blueprint.yaml`.
   gcluster regenerates deployment files on every run. Generated `.tf`, `.tfvars`
   and `.pkrvars.hcl` files have a `# DO NOT EDIT: Generated by gcluster.` line
   just below the license header — if you see that line, you are in the wrong file,
   even if a terraform error points at it. A failed `./gcluster deploy` prints a `Hint:` with
   the exact `create … -w` / `deploy … -w` command to re-run after fixing the
   source. When the user asks to **change settings** in a generated deployment folder (e.g., editing `<deployment_name>/primary/main.tf` or `terraform.tfvars` to change zone, node count, or a GCS state bucket, or running `terraform init`), refuse the direct `.tf` edit and `terraform` command, **check the workspace first** for an existing untracked `deployment.yaml` and `<deployment_name>/.ghpc/artifacts/expanded_blueprint.yaml` (to identify the source blueprint), update that `deployment.yaml` with the requested `vars:` overrides (and top-level `terraform_backend_defaults:` with `type: gcs` and `configuration.bucket` if a GCS bucket was given), and regenerate with `./gcluster create <blueprint.yaml> -d <deployment.yaml> -w`. Only ask the user for `deployment.yaml` or the source blueprint if none exists in the workspace. When the user reports a Terraform or `./gcluster deploy` **error** pointing
   at `<deployment_name>/primary/main.tf` (such as an unsupported argument or
   invalid setting line) — **even when `<deployment_name>/primary/main.tf` already
   exists on disk with a setting on that cited line** — `./gcluster deploy` is a
   mutating command under Rule 6: **NEVER** run `./gcluster deploy`,
   `./gcluster destroy`, or `terraform` (`init`, `validate`, `plan`, `apply`) to
   reproduce or test the error, **never** write scripts or inspect `modules/**/*.tf`
   or `community/modules/**/*.tf`, and **never** edit
   `<deployment_name>/primary/main.tf`. Check the module schema via
   `./gcluster explain <module> -f yaml`; if no untracked custom blueprint or
   `deployment.yaml` with an invalid setting exists on disk and the setting at that
   line in `examples/<blueprint>.yaml` is already valid in `./gcluster explain`
   (and `./gcluster expand` succeeds), **do NOT assume Terraform disagrees with
   `gcluster explain`, do NOT copy the tracked blueprint or delete valid settings,
   and do NOT emit a `[PROPOSED REMEDIATION PLAN]`** — reply directly in prose
   explaining that `<deployment_name>/primary/main.tf` is a **generated** file
   (`# DO NOT EDIT: Generated by gcluster`) overwritten on every run, explain that
   fixes belong in the source **blueprint** (e.g. `examples/hpc-slurm.yaml`) or
   **`deployment.yaml`** (`vars:`) followed by regenerating with
   `./gcluster create examples/<blueprint>.yaml -d deployment.yaml -w` (or
   `./gcluster create examples/<blueprint>.yaml -w`), and ask the user for the
   full error output or `deployment.yaml` they used. If
   you cannot reproduce any other reported error with `./gcluster create`, stop
   and ask the user for the full error output instead of searching further.
6. **Destructive or mutating actions require confirmation.** `./gcluster deploy`,
   `./gcluster destroy`, `./gcluster job submit`, `./gcluster job cancel`, and
   anything that creates or deletes cloud resources must
   be presented as a `[PROPOSED REMEDIATION PLAN]` and wait for the user to reply
   `yes`. The same applies to local destructive steps: deleting a deployment
   folder (`rm -rf <deployment_name>`), deleting user files or discarding user edits,
   and `git checkout` / `git restore` / `git reset` / `git clean` of user edits —
   present a `[PROPOSED REMEDIATION PLAN]` stating exactly what will be removed and
   wait for `yes`. Prefer `./gcluster create … -w` over deleting the deployment folder.
7. **Do only what was asked.** If you notice unrelated warnings or problems
   (deprecation notices, bugs in Go code, other blueprints), report them in your
   summary — do not fix, migrate or refactor them unprompted.

## Workflow

### 0. Ensure `./gcluster` is built

From the root of the repository, verify that `./gcluster` exists or build it via `make` (run `./gcluster` commands directly in the shell rather than delegating simple checks to subagents):

```bash
test -x ./gcluster || make
```

### 1. Find the right template

Use `./gcluster explain` or search [`skills/cluster-blueprint-assistant/references/catalog.yaml`](references/catalog.yaml):

```bash
# Find the exact or closest blueprint for given specs (plus missing module snippets):
./gcluster explain slurm a3u lustre
./gcluster explain gke a3-megagpu lustre

# Inspect a specific blueprint's required GCP APIs, customizable vars, and stages:
./gcluster explain examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml

# Inspect a module's required APIs, auto-injected inputs, and ready-to-copy 'use:' snippet
# (use -f yaml for full input/output metadata; do not pass --detail, which is not a valid flag):
./gcluster explain modules/file-system/managed-lustre
./gcluster explain modules/file-system/managed-lustre -f yaml

# For broken blueprints, running expand prints all validator errors and hints directly:
./gcluster expand <blueprint.yaml> --vars project_id=<project>

# Or search the catalog directly by scheduler, machine type, storage, or module source:
grep -n "scheduler: slurm" skills/cluster-blueprint-assistant/references/catalog.yaml
grep -n "managed-lustre" skills/cluster-blueprint-assistant/references/catalog.yaml
```

Each `blueprints[]` entry gives you: `path`, optional `deployment_path`,
`scheduler`, `description`, `machine_types`, optional `accelerators` and
`workloads`, the default `vars`, and the `modules` list with `id`, `source`,
`use`, and configured `settings` keys.
Prefer blueprints under `examples/` over `community/examples/` when both match.

### 2. Customize via deployment.yaml

Always create a **new, untracked** deployment file (e.g. `my-deployment.yaml` in
the repo root or `examples/<name>-deployment.yaml`) containing `ai_assisted: true` and `vars:`.
**Never modify git-tracked companion `*-deployment.yaml` files** under
`examples/` or `community/examples/` in place (`deployment_path` in the catalog
entry, such as `examples/gke-a4/gke-a4-deployment.yaml` or
`examples/hpc-slurm-h4d/hpc-slurm-h4d-deployment.yaml`) — read them only to
inspect variable names, and omit `terraform_backend_defaults` (`type: gcs`, `configuration.bucket`) unless
the user explicitly provides a GCS state bucket. Your new deployment file is
user-specific — do not commit it.

Only override keys that already exist in the blueprint's `vars:` (see the
catalog entry). Variable syntax is Cluster Toolkit's `$(vars.name)`, not
Terraform's `${var.name}`.

```yaml
# my-deployment.yaml
---
ai_assisted: true
vars:
  project_id: my-proj
  deployment_name: hpc-slurm-small
  region: us-central1
  zone: us-central1-a
  compute_node_count_dynamic_max: 4
  compute_machine_type: c2-standard-30
```

> Global `vars:` are injected into **every** module that declares an input with
> the same name and does not set it explicitly. That is why blueprint vars use
> prefixed names such as `compute_machine_type` /
> `compute_node_count_dynamic_max`. Never add an unprefixed variable name that
> is shared by multiple modules in the blueprint: in `hpc-slurm.yaml`, for
> example, a bare `machine_type` would silently apply to `slurm_controller` only
> (the one module that does not set it explicitly), whereas a bare input on a
> blueprint where multiple nodesets omit it would apply to all of them. Where
> every module already sets the input explicitly (such as `node_count_dynamic_max`
> or `enable_spot_vm` on all three nodesets in `hpc-slurm.yaml`), the bare name
> is injected nowhere and `gcluster` fails with
> `test_deployment_variable_not_used` instead.

#### Variable naming & override rules

Customizable vars in `examples/**/*.yaml` and `community/examples/**/*.yaml` follow
`<functional_role>_<exact_terraform_input>`; the suffix is the module input
the var feeds (`_machine_type`, `_node_count_dynamic_max`, `_node_count_static`,
`_static_node_count`, `_initial_node_count`, `_num_instances`, `_instance_count`,
`_autoscaling_total_min_nodes` / `_autoscaling_total_max_nodes`, `_size_gb`,
`_size_gib`, `_capacity_gb`, `_capacity_gib`, `_disk_size_gb`, `_disk_size`,
`_enable_spot_vm`, `_spot`), so no translation is needed between the var and
the setting. Every sizing, machine-type, storage-capacity and Spot knob on a
blueprint's worker compute and storage modules is exposed this way with a
concrete default equal to what the module would otherwise use (for example
`compute_enable_spot_vm: false` in `hpc-slurm.yaml`), so a user only ever
*has* to fill in `project_id`, `deployment_name`, `region` and `zone`.
Control-plane roles (`login_`, `controller_`, `system_node_pool_`) expose
machine type and disk size but never Spot: those modules do not accept it.

| Role prefix | Meaning |
| :--- | :--- |
| `compute_` | the worker nodes — a Slurm nodeset **or** a GKE node pool — when the blueprint has one |
| `login_` / `controller_` | the Slurm login / controller VM |
| `system_node_pool_` | the GKE system node pool (literal `gke-cluster` input) |
| `client_` | a plain VM that mounts storage (e.g. `pfs-managed-lustre-vm`) |
| `image_build_` | the temporary Packer VM that builds a custom image (`ml-slurm`, `ml-slurm-g4`) |
| `<partition>_` / `<pool>_` (`debug_`, `hp_pool_`, `local_ssd_pool_`, `n2_`, `h3_`, `a2_8_`) | one of several worker pools; the prefix is the partition / pool name the user already addresses |
| `homefs_`, `appsfs_`, `scratchfs_`, `sharefs_`, `lustre_`, `hyperdisk_<type>_`, `data_disk_` | storage, by mount role |

Rules the agent must respect:

- **Hardware-named partitions expose node counts and Spot, never
  `machine_type`** (`h3`, `a2`, `g2`, `n2`, `c2`, `c2d`, `c3`, `a2_8`,
  `a2_16`). The hardware name *is* the partition's identity; "run the `g2`
  partition on g4" is "add a partition" → Pattern 2/3 (copy the blueprint), not a
  `deployment.yaml` override. Functionally named pools (`compute`, `debug`,
  `hp_pool`, …) do expose `<role>_machine_type`.
- **Hardware-locked blueprints hardcode `machine_type` in the module.** The
  GKE and Slurm A3 / A4 / A4X blueprints have multi-NIC RDMA / GPUDirect wiring
  that only works with one shape, so they expose no compute machine-type var at
  all (the shape still appears in the catalog's `machine_types`). If the user
  asks for a different accelerator or machine series (e.g. `a3-highgpu-8g` on
  `gke-a4.yaml`), do **not** copy that blueprint and change `machine_type` —
  instead, use `./gcluster explain` to switch to the matching blueprint in
  `examples/` (e.g. `examples/gke-a3-highgpu/gke-a3-highgpu.yaml`). If no
  blueprint matches both the requested hardware and this blueprint's specialized
  features (e.g. inference gateway, DWS Flex Start queued provisioning, GCSFuse +
  Local SSD), stop and inform the user; do not silently drop those features and
  do not copy the blueprint to swap `machine_type`. Multi-shape
  families (`gke-h4d`, `gke-tpu-*`, `gke-g4*`, all CPU blueprints) expose
  `compute_machine_type` with an `# Allowed:` comment listing valid shapes.
- **Three override states & GKE `SPECIFIC_RESERVATION`.** `""` means "empty
  string" (e.g. `compute_reservation_name: ""` in `ml-slurm-g4` or
  `h4d_reservation_name: ""` in `hpc-slurm-h4d` = no reservation); `$(null)`
  means "unset this input" (e.g. `static_node_count` must be unset under
  flex-start); a bare `key:` in YAML or `key=` on `--vars` is **always an
  error** (`deployment variable "key" was not set`). That error means a **null
  value in your `-d` file or `--vars`** — even though the location gcluster
  prints is the blueprint's `vars:` line, the fix belongs in `deployment.yaml`,
  not in the template. Note: if a GKE blueprint hardcodes
  `consume_reservation_type: SPECIFIC_RESERVATION` (such as `gke-tpu-v5e.yaml` or
  `gke-a4x.yaml`) and the user did not provide a
  reservation name, do **not** set `reservation: ""` (which generates an invalid
  empty `SPECIFIC_RESERVATION`); stop and ask whether the user has a reservation
  name or wants a custom blueprint copy (`ai_assisted: true`) configured for
  on-demand, Spot, or DWS Flex Start.
- **Precedence:** blueprint `vars:` < `-d deployment.yaml` < `--vars`. Later
  sources override earlier ones key by key.
- **Only use keys present in the catalog entry's `vars`, and never silence
  validators.** A `deployment.yaml` key that is *not* in the blueprint's `vars:`
  but happens to match a module input (a typo such as `machine_type` instead of
  `compute_machine_type`, or `enable_spot_vm` on a blueprint that has no Spot
  var) is **silently injected into every module that lacks that setting** — no
  error is raised as long as at least one module can absorb it (only when *no*
  module can does `test_deployment_variable_not_used` fail). Check `vars` in the
  catalog before writing the key. Never run `gcluster` with `-l IGNORE` /
  `--validation-level IGNORE`, never pass `--skip-validators` (except for the
  documented offline `GHPC_MOCK_MACHINE_CONFIG` dry-run when unauthenticated),
  and never add `validation_level: IGNORE` or `skip: true` under `validators:`
  in a blueprint YAML to silence a failing validator (`test_machine_type_in_zone`,
  `test_zone_in_region`, `test_deployment_variable_not_used`, etc.).
- **The catalog is the contract, not this document.** Official `examples/`
  follow the `<role>_<terraform_input>` convention; `community/examples/` may
  still use older names (`cluster_size`, `size_gib`, bare `machine_type`, …).
  Do not memorize exceptions: always read the chosen blueprint's `vars` list in
  `catalog.yaml` before writing keys to `deployment.yaml` or `--vars`. If a
  machine type appears in the catalog's `machine_types` but the entry has no
  `*_machine_type` var, it is hardware-locked — do not try to override it.
- Companion `*-deployment.yaml` files whose blueprint exposes prefixed vars end
  with a commented `Optional overrides (defaults shown)` block listing them;
  copy those variable names into your own untracked `deployment.yaml` rather than
  editing the tracked companion file in place.

If the blueprint does not expose the setting you need in `vars:`, copy the
tracked template to a new file in the same directory with `ai_assisted: true`
(or edit your existing custom copy in place), add a new `vars:` key with the
current hardcoded value or module default as its default, reference it from the module via
`$(vars.<name>)`, and put the user's override value in `deployment.yaml` — this
keeps the existing behavior and leaves tracked templates untouched.

### 3. Validate

```bash
# Dry-run (no deployment folder written; writes expanded.yaml and still runs cloud validators if authenticated):
./gcluster expand examples/hpc-slurm.yaml -d my-deployment.yaml -o expanded.yaml

# Offline (only after normal ./gcluster expand fails due to missing GCP credentials, or when using a placeholder project_id; ADC works even if gcloud auth list is empty):
GHPC_MOCK_MACHINE_CONFIG='{}' ./gcluster expand examples/hpc-slurm.yaml -d my-deployment.yaml -o expanded.yaml \
  --skip-validators="test_project_exists,test_apis_enabled,test_region_exists,test_zone_exists,test_zone_in_region,test_quota_availability,test_machine_type_in_zone,test_reservation_exists,test_disk_type_in_zone"

# Full: writes the deployment folder and runs read-only cloud validators
# (project, APIs, zone, machine type, reservations). Use -w on re-runs instead
# of deleting the folder.
./gcluster create examples/hpc-slurm.yaml -d my-deployment.yaml -w
```

Read errors bottom-up. A path such as
`deployment_groups[0].modules[3].settings.machine_type` points at the blueprint
YAML — fix it there.

### 4. Common failures and where to fix them

| Symptom | Fix location |
| :--- | :--- |
| `a required setting "X" is missing from a module "Y"` | Read and follow the actionable `Hint:` line printed directly by `./gcluster expand <blueprint.yaml> --vars project_id=<project>` (e.g., `Consider adding module "private_service_access" to the "use" list`), check `module_wiring.<source>.recommended_use` in [`skills/cluster-blueprint-assistant/references/catalog.yaml`](references/catalog.yaml), or run `./gcluster explain <module>` (or `-f yaml`) and add that module to `Y`'s `use:` list—never run `grep -r`, `rg`, `find`, or shell/Python directory walks across `modules/` or `community/modules/` to search for `X`. |
| `module "X" was not used` | Add `X` to the `use:` list of its consumer (e.g. `private_service_access` → filestore / lustre). |
| `module "X" references unknown global variable "Y"` | Add `Y` under `vars:` in the blueprint or `deployment.yaml`. |
| `deployment variable "X" was not set` | A null value (`X:` with nothing after it) in **your** `-d` file or `--vars`, even though the location printed is the blueprint's `vars:` line. Set the value in `deployment.yaml`; do not edit the template. |
| `test_deployment_variable_not_used` / `the variable "X" was not used in this blueprint` | `X` is not a key in the blueprint's `vars:` and no module declares an input named `X` — almost always a typo'd key in `deployment.yaml`. Fix the key; never silence it with `-l IGNORE` or `--skip-validators`. |
| Reservation / machine type not found in zone | Change `zone` / `machine_type` in `vars:` or `deployment.yaml`. |
| User asks to edit `<deployment_name>/<group>/main.tf` or `terraform.tfvars` to change settings (zone, node count, GCS bucket) or run `terraform init` | Refuse the `.tf` edit and `terraform` command; check the workspace first for an untracked `deployment.yaml` and `<deployment_name>/.ghpc/artifacts/expanded_blueprint.yaml` (source blueprint), update `deployment.yaml` `vars:` (adding top-level `terraform_backend_defaults:` with `type: gcs` and `configuration.bucket` if a GCS bucket was given), and regenerate via `./gcluster create <blueprint.yaml> -d <deployment.yaml> -w` (only ask if no `deployment.yaml` or source blueprint exists). |
| Terraform or `gcluster deploy` error pointing at `<deployment_name>/<group>/main.tf` | `main.tf` is a generated file (`# DO NOT EDIT: Generated by gcluster`) overwritten on every run — never edit it, never run `./gcluster deploy` or `terraform` to reproduce the error, and never read `modules/**/*.tf` or `community/modules/**/*.tf` (see Core Rule 5). Verify the module schema with `./gcluster explain <module> -f yaml`; if the blueprint setting is already valid and no invalid untracked blueprint or `deployment.yaml` exists on disk, explain directly in prose (without copying the blueprint or emitting `[PROPOSED REMEDIATION PLAN]`) that fixes belong in the source blueprint or `deployment.yaml` followed by `./gcluster create ... -w`, and ask for the full error output or `deployment.yaml`. |

#### Module Wiring & Validation Discovery

Run `./gcluster explain <module>` (or `./gcluster explain <module> -f yaml`) to inspect required/optional inputs, `use:` wiring snippets, object schemas, deprecated settings, and input validation constraints (do not pass `--detail`, which is not a valid flag). Run `./gcluster expand <blueprint.yaml> --vars project_id=<project>` to surface all module, graph, and cross-field validation errors and `Hint:` suggestions directly—never read `.tf` files or run recursive searches (`grep -r`, `rg`, `find`, or shell/Python walks) across `modules/` or `community/modules/`.

### Day-2 Drift & reconfigure_ignores

When updating an existing cluster on Day-2, Terraform may either plan destructive replacement (`# forces replacement`) or ignore changes made in `deployment.yaml`. Use `reconfigure_ignores` in the module block to control Terraform's `lifecycle.ignore_changes`:

| Symptom | What Happened | What to Add |
| :--- | :--- | :--- |
| **Unwanted Downgrade or Recreation:** `terraform plan` says `forces replacement` on `placement_policy` or plans to downgrade `version`. | GKE auto-upgraded the cluster node pool, or metadata changed, and Terraform wants to downgrade or replace the node pool. | **Add to ignores (`attr`):**<br>```reconfigure_ignores:```<br>&nbsp;&nbsp;```google_container_node_pool:```<br>&nbsp;&nbsp;```  - version``` |
| **Changes Ignored:** You updated `labels` in `vars:` or `deployment.yaml`, but `terraform plan` says `No changes`. | The module hardcoded `ignore_changes = [node_config[0].labels]` to prevent Day-1 drift, blocking in-place updates. | **Subtract from ignores (`-attr`):**<br>```reconfigure_ignores:```<br>&nbsp;&nbsp;```google_container_node_pool:```<br>&nbsp;&nbsp;```  - "-node_config[0].labels"``` |

#### Common Recipes

```yaml
    reconfigure_ignores:
      google_container_node_pool:
        # Prevent node pool downgrade after GKE minor auto-upgrade:
        - version
        # Allow Day-2 node label updates to take effect in-place:
        - "-node_config[0].labels"
```

#### Discovery & Self-Diagnostics
- Run `./gcluster explain <module>` to inspect a module's default `Lifecycle Ignore Guardrails`.
- `gcluster` validates all entries: typos or subtracting non-existent attributes fail fast with actionable error messages listing available resources, ignores, and module source links.

### 5. Deploy (gated)

Present this and wait for explicit confirmation before running anything that
creates cloud resources:

```markdown
[PROPOSED REMEDIATION PLAN]
- **Target Resource**: deployment <deployment_name> in project <project_id>
- **Root Cause Identified**: <why this deployment / change is needed>
- **Proposed Action**: gcluster deploy examples/<blueprint>.yaml -d my-deployment.yaml
- **Blast Radius**: creates VPC, Filestore, Slurm controller/login/compute VMs (billable)
- **Confirmation Required**: Reply 'yes' to proceed.
```

## Out of scope

Debugging running VMs, Slurm jobs or GKE pods after deployment — use the
scheduler-specific skills (e.g. `gke-kueue-debugging`) for that.
