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

name: release-breaking-check
description: >
  Decide whether a Cluster Toolkit PR is release-breaking, i.e. whether an
  existing cluster can be upgraded just by re-running gcluster deploy. Use before
  adding a release-* label, when a PR touches modules, examples, pkg or cmd, or
  when asked if a change needs the release-breaking-changes label.
compatibility: "Requires git and python3. Read-only; never runs gcluster deploy or terraform."
metadata:
  author: GoogleCloudPlatform
  status: experimental
  domain: release
allowed-tools: Bash(git:diff) Bash(git:log) Bash(git:show) Bash(python3:skills/release-breaking-check/scripts/diff_scan.py)
---

# Release-breaking change check

> [!WARNING]
> This skill is experimental. It is strictly read-only: it never runs
> `gcluster deploy`, `terraform`, or `gh pr edit`, and it never changes labels.
> It produces a verdict and proposed text for a human to apply.

## What "release-breaking" means

**In one sentence:** A change is release-breaking if people who already have a
cluster can't upgrade it just by running `gcluster deploy` again.

### The situation

1. Someone built a cluster with an older version of the toolkit.
2. They get the newer version.
3. They run `gcluster deploy` again with the same blueprint. They change nothing
   on their side.

(When the version changes, gcluster asks you to add `--force`. That is expected,
so it doesn't count.)

### It is breaking if any of these happen

- **It fails.** gcluster stops with an error because of our change. Example: we
  renamed a setting their blueprint uses.
- **Something they have gets deleted or rebuilt.** Example: their shared file
  storage is deleted and created again, empty.
- **Their cluster starts to behave differently.** Example: we turned on a new
  feature by default, and it changes how their nodes work.
- **Something changes quietly.** Example: a setting they use is now ignored, and
  nothing warns them.
- **They have to do extra work.** Example: they must edit their blueprint or run
  extra commands first.

It only has to happen to some users, not all of them.

### It is not breaking if

- It only adds new things and leaves everything they have alone. Example: a new
  optional setting, a new module, or a new example.
- The only changes are harmless, like labels or version numbers.
- The error comes from their own project, not from our change. Example: they ran
  out of quota.
- It fixes something that was already broken.

If a new thing changes how existing parts behave, treat it as breaking. Example:
it installs new software on nodes that are already running.

### Example blueprints count too

Many people deploy our example blueprints directly. So changing an example can
break them too. Watch out for a changed value, a renamed module, or a renamed
variable.

### How to check

1. Deploy with the old version.
2. Run `gcluster deploy` again with the new version.
3. At the prompt, press `d` to see every planned change.

Look for three things: an error, "must be replaced", or "will be destroyed". If
any of them touches something that already exists, the change is breaking.

### If it is breaking

- Add the `release-breaking-changes` label (keep any other `release-*` label).
- Add short "upgrade steps" to the PR, so users know what to do.
- Make the steps work even for users who skip several versions.

### A real example

PR #5949 changed the default name of a reserved IP range. On older clusters,
`gcluster deploy` then planned to delete their shared file storage and create a
new, empty one. It showed no error. One customer lost their file storage this
way. That is a breaking change.

## Procedure

1. **Scan the diff for mechanical signals.**

   ```bash
   python3 skills/release-breaking-check/scripts/diff_scan.py --base <base-ref> --head <head-ref>
   # or, with a saved diff:
   python3 skills/release-breaking-check/scripts/diff_scan.py --diff-file pr.diff
   ```

   The script prints JSON: `signals[]` (id, file, detail, weight),
   `touched_examples[]`, `touched_modules[]`, `signal_count`, `severity_hint`.
   A signal is a *prompt to look*, not a verdict.

2. **For every signal, trace the effect on an EXISTING deployment** (not a fresh
   one). Read the surrounding Terraform or YAML with `git show <head>:<path>`
   and ask, in this order:
   - Would `gcluster expand`/`deploy` error for a user whose deployment file or
     blueprint still matches the old version? (removed/renamed variable, new
     required variable, tightened validation, removed module id or output)
   - Would Terraform replace or destroy a resource that already exists?
     (`name`, `image`, `machine_type`, `zone`, disk, network/subnet, reserved
     range, Helm/kubectl-apply release name, module `source` under the same
     `id`, script or template content rendered into metadata or GCS)
   - Would the running cluster behave differently without the user asking?
     (default flipped, version pinned higher, port or auth mode changed)

3. **Check example blueprints separately.** Any changed value, renamed module
   id, renamed or removed variable, or deleted file under `examples/` or
   `community/examples/` is breaking for users who deployed that example, even
   if the PR title says "test" or "ci".

4. **Eliminate false positives** using "It is not breaking if" above. Common
   ones: a new variable whose default equals the previous hardcoded value; an
   optional field added to a schema; a fix for something that never worked; a
   revert that cancels out within the same release; docs or tests only.

5. **If you cannot decide from the diff,** return `NEEDS_PLAN_CHECK` and say
   which blueprint to deploy old→new and what to look for in the plan (`d`).

6. **Write the output block below and nothing else.** CI parses it.

## Output format

```text
<!-- release-breaking-check -->
VERDICT: BREAKING | NOT_BREAKING | NEEDS_PLAN_CHECK
CONFIDENCE: high | medium | low
SEVERITY: high | medium | low | none
WHY: <1-3 sentences; name the file and attribute and what happens on redeploy>
SIGNALS: <comma-separated signal ids from diff_scan, or none>
UPGRADE_STEPS:
- <step that works even when skipping several versions>
REDEPLOY_IMPACT: <one line; copied verbatim into the release notes>
```

Rules for the block:

- `SEVERITY: high` = data destroyed or a core long-lived resource rebuilt
  (file system, Slurm controller, GKE cluster). `medium` = deploy fails, user
  must act, or node pools / login nodes / networks / in-cluster objects are
  rebuilt. `low` = in-place behaviour change. `none` only with `NOT_BREAKING`.
- `UPGRADE_STEPS` and `REDEPLOY_IMPACT` are required when `VERDICT: BREAKING`;
  write `- none` / `none` otherwise.
- Never propose running `gcluster deploy`, `terraform`, or changing labels
  yourself. If a human asks you to, answer with a `[PROPOSED REMEDIATION PLAN]`
  and wait for confirmation.

## Where this is used

- **Locally**, by contributors and coding agents before opening a PR.
- **In CI**, by `.github/workflows/pr-breaking-change-check.yml`, which runs
  `diff_scan.py` on every PR and, when it finds signals, asks a model to follow
  this skill and posts the output block as a PR comment. The check compares the
  verdict with the PR's labels and description.
- **Overrides**: a maintainer (not the PR author) can add
  `breaking-check/not-breaking` or `breaking-check/plan-verified` to settle a
  disputed verdict; say why in a comment.
