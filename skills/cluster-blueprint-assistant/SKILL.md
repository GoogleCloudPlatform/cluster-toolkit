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
allowed-tools: Bash(make) Bash(./gcluster expand:*) Bash(./gcluster create:*) Bash(gcluster expand:*) Bash(gcluster create:*)
allowed_read_only_commands:
  - "make"
  - "./gcluster expand"
  - "./gcluster create"
  - "gcluster expand"
  - "gcluster create"
---

# Cluster Toolkit Blueprint Assistant

> [!WARNING]
> This skill is experimental. Always notify the user before executing diagnostic steps and explicitly request confirmation for any proposed remediations.

## Core rules (read before touching any YAML)

1. **Start from the catalog, never from scratch.** Read
   [`references/catalog.yaml`](references/catalog.yaml) to find an existing
   blueprint under `examples/` or `community/examples/` that already matches the
   request. Do not grep `.tf` files to discover modules; the catalog's
   `module_wiring` section lists a module's `required_inputs`, which module
   sources satisfy them (`recommended_use`), and which modules consume its
   outputs (`consumed_by`). If the request is a question ("do we
   have…", "can I get…", "what's in…"), answer it from the catalog and
   `examples/README.md` and confirm the approach with the user **before**
   editing anything.
2. **Customize through a `deployment.yaml`, not module blocks.** If the request
   only changes project, region/zone, node counts, machine types,
   Spot/reservation, or storage size, put the user's values in a
   `deployment.yaml` and pass it with `-d`. For templates under `examples/` or
   `community/examples/`, only touch the blueprint's `vars:` when the knob does
   not exist yet: add it with a default **equal to the value that was
   hardcoded**, reference it via `$(vars.<name>)`, and keep the user's requested
   value in `deployment.yaml` only. (If the blueprint is the user's own copy,
   editing its `vars:` directly is fine.) Do **not** add, remove, rename or edit
   `deployment_groups[].modules[]` entries for these changes.
3. **Only edit module blocks when adding or removing capability** (e.g. adding a
   Managed Lustre instance or a second node pool) — and do it on a **copy**, not
   the shared template: copy the blueprint to a new file in the **same
   directory** (e.g. `examples/gke-a4/my-gke-a4.yaml`) or copy the whole folder,
   because blueprints reference sibling files via `$(ghpc_stage("./…"))`. Never
   modify existing template files under `examples/` or `community/examples/` in
   place (once you create a copy, edit that copy directly for follow-ups). Copy
   the module block from a blueprint in the catalog that already has it, keep
   its `use:` list, and check `module_wiring.<source>.required_inputs` to
   confirm every required input is satisfied by a module in `use:`.
4. **Mark agent-authored blueprints.** Add `ai_assisted: true` at the top level
   of any blueprint you create or modify.
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

```bash
# Search the catalog by scheduler, machine type, storage, or module source.
grep -n "scheduler: slurm" skills/cluster-blueprint-assistant/references/catalog.yaml
grep -n "managed-lustre" skills/cluster-blueprint-assistant/references/catalog.yaml
```

Each `blueprints[]` entry gives you: `path`, optional `deployment_path`,
`scheduler`, `description`, `machine_types`, the default `vars`, and the
`modules` list with `id`, `source` and `use`. Prefer blueprints under
`examples/` over `community/examples/` when both match.

> The catalog is currently a starter covering `examples/hpc-slurm.yaml` and
> `examples/gke-a4/gke-a4.yaml`. If neither fits, browse
> [`examples/README.md`](../../examples/README.md) for the right template and
> still follow the rules below.

### 2. Customize via deployment.yaml

Create the deployment file next to the blueprint (e.g.
`examples/hpc-slurm-deployment.yaml`, matching the `gke-a4/*-deployment.yaml`
convention) or in the repo root. It is user-specific — do not commit it.

Only override keys that already exist in the blueprint's `vars:` (see the
catalog entry). Variable syntax is Cluster Toolkit's `$(vars.name)`, not
Terraform's `${var.name}`.

```yaml
# my-deployment.yaml
---
vars:
  project_id: my-proj
  deployment_name: hpc-slurm-spot
  region: us-central1
  zone: us-central1-a
  compute_node_count_max: 4
  compute_enable_spot_vm: true
```

> Global `vars:` are injected into **every** module that declares an input with
> the same name and does not set it explicitly. That is why blueprint vars use
> prefixed names such as `compute_machine_type` / `compute_enable_spot_vm`.
> Never add an unprefixed variable name that is shared by multiple modules in
> the blueprint: for example, `machine_type` would silently apply to all
> nodesets, `slurm_login`, and `slurm_controller`, and `enable_spot_vm` would
> apply to all three nodesets (`debug_nodeset`, `compute_nodeset`, `h3_nodeset`).

If the blueprint does not expose the setting you need in `vars:`, add a new
`vars:` key with the current hardcoded value as its default and reference it
from the module via `$(vars.<name>)` — this keeps the existing behaviour and
makes the knob reusable.

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
