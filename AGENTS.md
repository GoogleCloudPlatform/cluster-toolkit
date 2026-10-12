# Agent Instructions for Cluster Toolkit

This file is read by coding agents (Gemini CLI, Claude Code, Cursor, Copilot, etc.).

## Before creating, editing, validating or deploying a blueprint

1. Read [`skills/cluster-blueprint-assistant/SKILL.md`](skills/cluster-blueprint-assistant/SKILL.md).
2. Look up templates and module wiring in
   [`skills/cluster-blueprint-assistant/references/catalog.yaml`](skills/cluster-blueprint-assistant/references/catalog.yaml)
   instead of grepping `.tf` files.

## Non-negotiable rules

- Start from an existing blueprint in `examples/` or `community/examples/`; never write one from scratch.
- Change project, zone, node counts, machine types, Spot/reservations or storage size via `vars:` or a `deployment.yaml` (`-d`). Do not edit module blocks for these.
- Add `ai_assisted: true` at the top level of any blueprint you create or modify.
- Validate with `gcluster expand` / `gcluster create`. On failure, fix the blueprint YAML or `deployment.yaml` and re-run gcluster.
- Never edit generated files under `<deployment_name>/` and never run `terraform` directly.
- `gcluster deploy` / `gcluster destroy` require explicit user confirmation first.

Other skills live under [`skills/`](skills/README.md).
