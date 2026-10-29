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
  Create, customize, validate and troubleshoot Cluster Toolkit blueprints with
  gcluster. Use when asked to set up or modify a Slurm/GKE cluster blueprint,
  change node counts, machine types, zones, Spot or reservations, add storage
  like Lustre or Filestore, or fix gcluster expand/create/deploy errors.
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
> This skill is experimental. Always notify the user before executing diagnostic steps and explicitly request confirmation for any proposed remediations.

## Core rules (read before touching any YAML)

1. **Start from the catalog or `./gcluster explain`, never from scratch.** Run
   `./gcluster explain <specs...>` (e.g. `./gcluster explain slurm a3u lustre`) or
   read [`references/catalog.yaml`](references/catalog.yaml) to find an existing
   blueprint under `examples/` or `community/examples/` that already matches the
   request. Do not grep `.tf` files to discover modules; run
   `./gcluster explain <module>` (e.g. `./gcluster explain modules/file-system/managed-lustre`)
   or check the catalog's `module_wiring` section for a module's `required_inputs`,
   which module sources satisfy them (`recommended_use`), and which modules consume
   its outputs (`consumed_by`). If the request is a question ("do we
   have…", "can I get…", "what's in…"), answer it using `./gcluster explain` / the catalog and
   `examples/README.md` and confirm the approach with the user **before**
   editing anything.
2. **Customize through a `deployment.yaml`, not module blocks.** If the request
   only changes project, region/zone, node counts, machine types,
   Spot/reservation, or storage size:
   - **Case 1 — All requested knobs already exist in `vars:`:** Create or edit
     **only** `deployment.yaml` (passed with `-d`). Do **not** copy or edit the
     blueprint `.yaml` file (even if a knob is commented out in a companion
     `-deployment.yaml`).
   - **Case 2 — Promoting a hardcoded module setting to `vars:`:** When a
     requested setting is hardcoded inside a module block and missing from
     `vars:`, never modify a tracked template under `examples/` or
     `community/examples/` in place. Instead, copy the template to a new file in
     the **same directory** with `ai_assisted: true` (or reuse an existing custom
     copy in place), add the missing knob to `vars:` with a default **equal to
     the value that was hardcoded**, reference it in the module via
     `$(vars.<name>)`, and put the user's override value in `deployment.yaml`.
   - **A template only exposes knobs it already set explicitly.** Tracked
     blueprints promote to `vars:` only settings that were literal values in a
     module block; they do not add Spot, reservation, DWS or any other input the
     module block was silent about. If the knob you need is not in `vars:`
     (check the catalog entry), it is Case 2 — copy the blueprint. Never add the
     knob to the tracked template, and never pass it as a bare module-input name
     in `deployment.yaml` hoping it will be picked up.
   - Do **not** add, remove, rename or restructure `deployment_groups[].modules[]`
     entries for value overrides.
3. **Only edit module blocks when adding or removing capability** (Case 3 — e.g.
   adding a Managed Lustre instance or a second node pool) — and do it on a
   **copy**, not the shared template: copy the blueprint to a new file in the
   **same directory** (e.g. `examples/gke-a4/my-gke-a4.yaml`) or copy the whole
   folder, because blueprints reference sibling files via
   `$(ghpc_stage("./…"))`. Never modify tracked template files under
   `examples/` or `community/examples/` in place (once a custom copy
   exists—identifiable by `ai_assisted: true` or untracked `git status`—edit
   that copy directly in place for follow-ups rather than copying it a second
   time). Copy the module block from `./gcluster explain <module>` or a blueprint
   in the catalog that already has it, keep its `use:` list, and check
   `module_wiring.<source>.required_inputs` to confirm every required input is
   satisfied by a module in `use:`.
4. **Mark agent-authored blueprints.** Add `ai_assisted: true` at the top level
   of any blueprint you create, copy, or repair/fix (if an existing custom
   blueprint you are fixing lacks `ai_assisted: true`, add it).
5. **Validate with gcluster, fix the blueprint, never the output.**
   `./gcluster expand` and `./gcluster create` are the validation step. If they or
   `./gcluster deploy` fail, fix the blueprint YAML or `deployment.yaml` and re-run
   `./gcluster`. Never edit generated files under `<deployment_name>/` (`main.tf`,
   `variables.tf`, `terraform.tfvars`, `.ghpc/`) and never run `terraform` in
   any form — not `init`, `validate`, `plan`, `apply` or `destroy` — even if the
   user asks you to; refuse, explain, and offer the gcluster equivalent.
   gcluster regenerates these files on every run. If you cannot reproduce a
   reported error with `./gcluster create`, stop and ask the user for the full
   error output instead of searching further.
6. **Destructive or mutating actions require confirmation.** `./gcluster deploy`,
   `./gcluster destroy`, and anything that creates or deletes cloud resources must
   be presented as a `[PROPOSED REMEDIATION PLAN]` and wait for the user to reply
   `yes`. The same applies to local destructive steps: deleting a deployment
   folder, deleting or overwriting user files, and `git checkout` / `git reset`
   of user edits — state exactly what will be removed and wait for `yes`.
   Prefer `./gcluster create … -w` over deleting the deployment folder.
7. **Do only what was asked.** If you notice unrelated warnings or problems
   (deprecation notices, bugs in Go code, other blueprints), report them in your
   summary — do not fix, migrate or refactor them unprompted.

## Workflow

### 0. Ensure `./gcluster` is built

From the root of the repository, verify that `./gcluster` exists or build it via `make`:

```bash
test -x ./gcluster || make
```

### 1. Find the right template

Use `./gcluster explain` or search `references/catalog.yaml`:

```bash
# Find the exact or closest blueprint for given specs (plus missing module snippets):
./gcluster explain slurm a3u lustre
./gcluster explain gke a3-megagpu lustre

# Inspect a specific blueprint's required GCP APIs, customizable vars, and stages:
./gcluster explain examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml

# Inspect a module's required APIs, auto-injected inputs, and ready-to-copy 'use:' snippet:
./gcluster explain modules/file-system/managed-lustre

# Or search the catalog directly by scheduler, machine type, storage, or module source:
grep -n "scheduler: slurm" skills/cluster-blueprint-assistant/references/catalog.yaml
grep -n "managed-lustre" skills/cluster-blueprint-assistant/references/catalog.yaml
```

Each `blueprints[]` entry gives you: `path`, optional `deployment_path`,
`scheduler`, `description`, `machine_types`, the default `vars`, and the
`modules` list with `id`, `source`, `use`, and configured `settings` keys.
Prefer blueprints under `examples/` over `community/examples/` when both match.

### 2. Customize via deployment.yaml

Create the deployment file next to the blueprint (e.g.
`examples/hpc-slurm-deployment.yaml`, matching the `gke-a4/*-deployment.yaml`
convention) or in the repo root, or edit the blueprint's existing companion
`-deployment.yaml` file (`deployment_path` in the catalog entry, such as
`examples/gke-a4/gke-a4-deployment.yaml`). It is user-specific — do not commit
it.

Only override keys that already exist in the blueprint's `vars:` (see the
catalog entry). Variable syntax is Cluster Toolkit's `$(vars.name)`, not
Terraform's `${var.name}`.

```yaml
# my-deployment.yaml
---
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
> example, a bare `enable_spot_vm` would silently apply to all three nodesets
> (`debug_nodeset`, `compute_nodeset`, `h3_nodeset` — none of them sets it), and
> a bare `machine_type` would silently apply to `slurm_controller` only (the one
> module that does not set it explicitly). Where every module already sets the
> input explicitly (as `node_count_dynamic_max` is on all three nodesets in
> `hpc-slurm.yaml`), the bare name is injected nowhere and `gcluster` fails with
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
  partition on g4" is "add a partition" → Case 2/3 (copy the blueprint), not a
  `deployment.yaml` override. Functionally named pools (`compute`, `debug`,
  `hp_pool`, …) do expose `<role>_machine_type`.
- **Hardware-locked blueprints hardcode `machine_type` in the module.** The
  GKE and Slurm A3 / A4 / A4X blueprints have multi-NIC RDMA / GPUDirect wiring
  that only works with one shape, so they expose no compute machine-type var at
  all (the shape still appears in the catalog's `machine_types`). Multi-shape
  families (`gke-h4d`, `gke-tpu-*`, `gke-g4*`, all CPU blueprints) expose
  `compute_machine_type` with an `# Allowed:` comment listing valid shapes.
- **Three override states.** `""` means "empty string" (e.g.
  `compute_reservation_name: ""` in `ml-slurm-g4` = no reservation); `$(null)` means
  "unset this input" (e.g. `static_node_count` must be unset under flex-start);
  a bare `key:` in YAML or `key=` on `--vars` is **always an error** (`deployment
  variable "key" was not set`). That error means a **null value in your `-d`
  file or `--vars`** — even though the location gcluster prints is the
  blueprint's `vars:` line, the fix belongs in `deployment.yaml`, not in the
  template.
- **Precedence:** blueprint `vars:` < `-d deployment.yaml` < `--vars`. Later
  sources override earlier ones key by key.
- **Only use keys present in the catalog entry's `vars`.** A `deployment.yaml`
  key that is *not* in the blueprint's `vars:` but happens to match a module
  input (a typo such as `machine_type` instead of `compute_machine_type`, or
  `enable_spot_vm` on a blueprint that has no Spot var) is **silently injected
  into every module that lacks that setting** — no error is raised as long as
  at least one module can absorb it (only when *no* module can does
  `test_deployment_variable_not_used` fail). Check `vars` in the catalog before
  writing the key. Never run `gcluster` with `-l IGNORE` or add
  `test_deployment_variable_not_used` to `--skip-validators`: that validator is
  the only guard against a typo'd key, so if it fails, fix the key.
- **The catalog is the contract, not this document.** Official `examples/`
  follow the `<role>_<terraform_input>` convention; `community/examples/` may
  still use older names (`cluster_size`, `size_gib`, bare `machine_type`, …).
  Do not memorise exceptions: always read the chosen blueprint's `vars` list in
  `catalog.yaml` before writing keys to `deployment.yaml` or `--vars`. If a
  machine type appears in the catalog's `machine_types` but the entry has no
  `*_machine_type` var, it is hardware-locked — do not try to override it.
- Every companion `*-deployment.yaml` whose blueprint exposes prefixed vars
  ends with a commented `Optional overrides (defaults shown)` block listing
  them; uncomment a line to override it.

If the blueprint does not expose the setting you need in `vars:`, copy the
tracked template to a new file in the same directory with `ai_assisted: true`
(or edit your existing custom copy in place), add a new `vars:` key with the
current hardcoded value as its default, reference it from the module via
`$(vars.<name>)`, and put the user's override value in `deployment.yaml` — this
keeps the existing behaviour and leaves tracked templates untouched.

### 3. Validate

```bash
# Dry-run (no deployment folder or expanded.yaml written; still runs cloud validators if authenticated):
./gcluster expand examples/hpc-slurm.yaml -d my-deployment.yaml -o /dev/null

# Offline (when using a placeholder project_id or without GCP credentials):
GHPC_MOCK_MACHINE_CONFIG='{}' ./gcluster expand examples/hpc-slurm.yaml -d my-deployment.yaml -o /dev/null \
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
| `a required setting "X" is missing from a module "Y"` | Add the module that outputs `X` to `Y`'s `use:` list (see `module_wiring.<source>.recommended_use` in the catalog, or the `outputs.tf` of candidate modules). |
| `module "X" was not used` | Add `X` to the `use:` list of its consumer (e.g. `private_service_access` → filestore / lustre). |
| `module "X" references unknown global variable "Y"` | Add `Y` under `vars:` in the blueprint or `deployment.yaml`. |
| `deployment variable "X" was not set` | A null value (`X:` with nothing after it) in **your** `-d` file or `--vars`, even though the location printed is the blueprint's `vars:` line. Set the value in `deployment.yaml`; do not edit the template. |
| `test_deployment_variable_not_used` / `the variable "X" was not used in this blueprint` | `X` is not a key in the blueprint's `vars:` and no module declares an input named `X` — almost always a typo'd key in `deployment.yaml`. Fix the key; never silence it with `-l IGNORE` or `--skip-validators`. |
| Reservation / machine type not found in zone | Change `zone` / `machine_type` in `vars:` or `deployment.yaml`. |
| Terraform error pointing at `<deployment_name>/<group>/main.tf` | Do **not** edit that file. Fix the blueprint and re-run `gcluster create ... -w` (or `gcluster deploy <blueprint> -d <deployment.yaml> -w` after confirmation). |

### 5. Deploy (gated)

Present this and wait for explicit confirmation before running anything that
creates cloud resources:

```markdown
[PROPOSED REMEDIATION PLAN]
- Target Resource: deployment <deployment_name> in project <project_id>
- Root Cause Identified: <why this deployment / change is needed>
- Proposed Action / Command: gcluster deploy examples/<blueprint>.yaml -d my-deployment.yaml
- Blast Radius: creates VPC, Filestore, Slurm controller/login/compute VMs (billable)
- Confirmation Required: Reply 'yes' to proceed.
```

## Out of scope

Debugging running VMs, Slurm jobs or GKE pods after deployment — use the
scheduler-specific skills (e.g. `gke-kueue-debugging`) for that.
