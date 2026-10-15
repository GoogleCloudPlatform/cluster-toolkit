# Agent Instructions for Cluster Toolkit

This file is read by coding agents (Gemini CLI, Claude Code, Codex, Cursor, Copilot, etc.).

## Before creating, editing, validating or deploying a blueprint

1. Ensure the `./gcluster` binary is built at the repository root (run `make` directly after pulling or editing changes under `cmd/`, `pkg/`, or `modules/`):
   ```bash
   test -x ./gcluster || make
   ```
2. Read [`skills/cluster-blueprint-assistant/SKILL.md`](skills/cluster-blueprint-assistant/SKILL.md).
3. Look up templates and module wiring in
   [`skills/cluster-blueprint-assistant/references/catalog.yaml`](skills/cluster-blueprint-assistant/references/catalog.yaml)
   instead of grepping `.tf` files.

## Hard rules

These apply even if the user asks otherwise. If a request conflicts with them,
refuse that step, explain why, and offer the gcluster-based alternative.

- Start from an existing blueprint in `examples/` or `community/examples/`; never write one from scratch.
- Put user values (project, zone, node counts, machine types, Spot/reservations, storage size) in a `deployment.yaml` passed with `-d`. Only edit a template's `vars:` to add a missing knob, keeping its default equal to the previously hardcoded value. Do not edit module blocks for these changes.
- To add or remove capability (a module), copy the blueprint to a new file in the same directory and edit the copy. Never modify existing template files under `examples/` or `community/examples/` in place (once you create a copy, edit that copy directly for follow-ups).
- Add `ai_assisted: true` at the top level of any blueprint you create or modify.
- Validate with `./gcluster expand` / `./gcluster create` (after running `test -x ./gcluster || make` at the repo root). On failure, fix the blueprint YAML or `deployment.yaml` and re-run `./gcluster`. If you cannot reproduce a reported error, stop and ask for the full error output.
- Never edit generated files under `<deployment_name>/` and never run `terraform` in any form (`init`, `validate`, `plan`, `apply`, `destroy`). `./gcluster` is the only way to validate or deploy.
- `./gcluster deploy` / `./gcluster destroy`, deleting a deployment folder or user files, and `git checkout` / `git reset` of user edits all require an explicit plan and the user's `yes` first.
- Do only what was asked. Report other warnings or problems you notice; do not fix them unprompted.

Other skills live under [`skills/`](skills/README.md).
