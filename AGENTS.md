# Agent Instructions for Cluster Toolkit

This file is read by coding agents (Gemini CLI, Claude Code, Codex, Cursor, Copilot, etc.).

## Before answering questions about, creating, editing, validating, or deploying a blueprint

1. Ensure the `./gcluster` binary is built at the repository root (run `make` directly after pulling or editing changes under `cmd/`, `pkg/`, or `modules/`):
   ```bash
   test -x ./gcluster || make
   ```
2. **Mandatory on the first turn of any blueprint task:** Read [`skills/cluster-blueprint-assistant/SKILL.md`](skills/cluster-blueprint-assistant/SKILL.md) before writing files or answering blueprint questions.
3. Start with `./gcluster explain` (e.g. `./gcluster explain`, `./gcluster explain <specs...>`, `./gcluster explain <blueprint.yaml>`, or `./gcluster explain <module>`),
   [`skills/cluster-blueprint-assistant/references/catalog.yaml`](skills/cluster-blueprint-assistant/references/catalog.yaml),
   and [`llms.txt`](llms.txt) / [`llms-full.txt`](llms-full.txt)
   to discover templates, required GCP APIs, customizable `vars:`, and `use:` module wiring instead of running `ls`, `find`, or `grep` across `examples/` or `.tf` files.

## Hard rules

These apply even if the user asks otherwise. If a request conflicts with them,
refuse that step, explain why, and offer the gcluster-based alternative.

- Start from an existing blueprint in `examples/` or `community/examples/`; never write one from scratch.
- Put user values (project, zone, node counts, machine types, Spot/reservations, storage size) in a **new, untracked** `deployment.yaml` (or `<name>-deployment.yaml`) passed with `-d`, containing only `vars:` (omit `terraform_backend_defaults: type: gcs` unless the user provides a GCS state bucket). When all requested knobs already exist in the blueprint's `vars:`, create or edit **only** that untracked deployment file—do not copy or edit the blueprint `.yaml` file, and **never modify git-tracked companion `*-deployment.yaml` files** under `examples/` or `community/examples/` in place. Do not edit module blocks for value overrides. On casual or vague requests (e.g. a "small" or "cheap" cluster with Spot VMs and N nodes), override **only** the requested Spot and node-count variables in `deployment.yaml`—keep the blueprint's default machine types (do not replace `c2-standard-60` with `e2-standard-2`, which breaks `gvnic_enabled`) and do not remove partitions or storage modules unless explicitly requested.
- Never modify any git-tracked files under `examples/` or `community/examples/` in place. Only copy a tracked template to a new file in the same directory when (a) promoting a hardcoded module setting to `vars:` (keeping the new var's default equal to the previously hardcoded value, referencing it via `$(vars.<name>)`, and putting the user's override in `deployment.yaml`) or (b) adding or removing a module. Once a custom copy exists—identifiable by `ai_assisted: true` or untracked `git status`—edit that copy directly in place for follow-ups rather than copying it a second time.
- Respect `# Hardware-locked:` blueprints and mutually exclusive consumption models:
  - If a blueprint has a `# Hardware-locked:` comment on `machine_type` (e.g. `gke-a4.yaml` locked to `a4-highgpu-8g`) and the user asks for a different accelerator or machine series (e.g. `a3-highgpu-8g`), do **not** copy that blueprint and swap `machine_type` (its RDMA/GPUDirect networking is specific to that hardware). Instead, use `./gcluster explain` to switch to the matching blueprint in `examples/` (e.g. `examples/gke-a3-highgpu/gke-a3-highgpu.yaml`).
  - Spot VMs (`spot` / `enable_spot_vm`), DWS Flex Start (`enable_flex_start` / `dws_flex`), and Reservations (`reservation` / `SPECIFIC_RESERVATION`) are mutually exclusive on the same node pool/nodeset. Never combine them, and **never silently disable** a blueprint's existing consumption model (such as setting `enable_flex_start: false` on a DWS Flex Start blueprint when the user asks to add Spot VMs). Stop, explain the conflict, and ask the user which single consumption model they want.
  - If a GKE blueprint hardcodes `consume_reservation_type: SPECIFIC_RESERVATION` (e.g. `gke-tpu-v5e.yaml`) and the user did not provide a reservation name, do not set `reservation: ""`; stop and ask whether they have a reservation name or want a custom blueprint copy (`ai_assisted: true`) configured for on-demand/Spot/DWS Flex Start.
- Add `ai_assisted: true` at the top level of any blueprint you create, copy, or repair/fix (if an existing custom blueprint you are fixing lacks `ai_assisted: true`, add it).
- Validate **only** with `./gcluster expand` / `./gcluster create` (after running `test -x ./gcluster || make` at the repo root). Once `./gcluster create` or `./gcluster expand` succeeds, **stop immediately**—validation is complete. Never bypass validators using CLI flags (`-l IGNORE`, `--validation-level IGNORE`, `--skip-validators`) or YAML fields (`validation_level: IGNORE`, or `skip: true` under `validators:`) unless the user explicitly asks to skip a specific validator by name. If a zone or machine-type check (`test_machine_type_in_zone`, `test_zone_in_region`) fails, stop and ask the user for a supported zone/machine type. If you cannot reproduce a reported error, stop and ask for the full error output.
- Never edit generated files in a deployment folder (it has a `.ghpc/` folder; its `.tf`, `.tfvars` and `.pkrvars.hcl` files have a `# DO NOT EDIT: Generated by gcluster` line), even if a terraform error points at a line in one. Fix the blueprint or `deployment.yaml` and re-run `./gcluster create ... -w`.
- **Never run `terraform` in any form** (`init`, `validate`, `plan`, `apply`, `destroy`, `fmt`); `./gcluster` is the only way to validate or deploy. `./gcluster` does not have a dry-run `plan` subcommand—if asked for a dry-run plan or resource preview before deploying, never run `terraform plan` or pipe `yes n | ./gcluster deploy`; instead use `./gcluster explain <blueprint.yaml>` and inspect `<deployment_dir>/.ghpc/artifacts/expanded_blueprint.yaml`.
- **Confirmation gate (`[PROPOSED REMEDIATION PLAN]` + explicit `yes` required):** You must present a `[PROPOSED REMEDIATION PLAN]` and wait for the user's explicit `yes` before executing ANY of:
  1. `./gcluster deploy` or `./gcluster destroy` (creating or destroying cloud resources),
  2. Deleting a deployment folder (`rm -rf <deployment_dir>`) or deleting any user `.yaml` files (prefer `./gcluster create ... -w` to overwrite in place instead of deleting),
  3. Reverting user edits via `git checkout`, `git restore`, `git reset`, or `git clean`.
- Do only what was asked. Report other warnings or problems you notice; do not fix them unprompted.

Other skills live under [`skills/`](skills/README.md).
