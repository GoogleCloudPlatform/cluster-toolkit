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
- Put user values (project, zone, node counts, machine types, Spot/reservations, storage size) in a `deployment.yaml` passed with `-d`. When all requested knobs already exist in the blueprint's `vars:`, create or edit **only** `deployment.yaml`—do not copy or edit the blueprint `.yaml` file (even if a knob is commented out in a companion `-deployment.yaml`). Do not edit module blocks for value overrides.
- Never modify tracked template files under `examples/` or `community/examples/` in place. Only copy a tracked template to a new file in the same directory when (a) promoting a hardcoded module setting to `vars:` (keeping the new var's default equal to the previously hardcoded value, referencing it via `$(vars.<name>)`, and putting the user's override in `deployment.yaml`) or (b) adding or removing a module. Once a custom copy exists—identifiable by `ai_assisted: true` or untracked `git status`—edit that copy directly in place for follow-ups rather than copying it a second time.
- Add `ai_assisted: true` at the top level of any blueprint you create, copy, or repair/fix (if an existing custom blueprint you are fixing lacks `ai_assisted: true`, add it).
- Validate with `./gcluster expand` / `./gcluster create` (after running `test -x ./gcluster || make` at the repo root). On failure, fix the blueprint YAML or `deployment.yaml` and re-run `./gcluster`. If you cannot reproduce a reported error, stop and ask for the full error output.
- Never edit generated files under `<deployment_name>/` and never run `terraform` in any form (`init`, `validate`, `plan`, `apply`, `destroy`). `./gcluster` is the only way to validate or deploy.
- `./gcluster deploy` / `./gcluster destroy`, deleting a deployment folder or user files, and `git checkout` / `git reset` of user edits all require an explicit plan and the user's `yes` first.
- Do only what was asked. Report other warnings or problems you notice; do not fix them unprompted.

Other skills live under [`skills/`](skills/README.md).

- 2026-10-28 10:51Z: Addressed gemini-code-assist review comments on PR #6561:
  1. Implemented `isNilOrNull(v cty.Value) bool { return v == cty.NilVal || v.IsNull() }` in `cmd/create.go`.
  2. Applied `isNilOrNull` in `settingChanged` to prevent false-positive drift warnings when optional settings toggle between omitted and explicitly null.
  3. Applied `isNilOrNull` in `getSettingVal` so fallback aliases are evaluated when primary settings are explicitly set to null.
  4. Added Unit Test Cases 11 (explicit null vs absent) and 12 (explicit null alias fallback) in `cmd/create_test.go`. Verified all 12 test cases pass cleanly.
  5. Committed commit `2e2ef853b` and pushed to `origin/swarnabm/gcluster-day2-ignore-warnings`.
  6. Synced Tab 2 of Google Doc `1n2CxfaF_gtc82ZsYFqPbzjdenesjQnM509jX5onyAnw` with the updated code.
