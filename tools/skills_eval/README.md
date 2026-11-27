# Skills Evaluation Framework (`tools/skills_eval`)

`tools/skills_eval` implements the offline and `--live` evaluation harness behind
[`../run_eval.py`](../run_eval.py). For skill authoring rules, `EVAL.yaml` fields, and CLI usage
examples, see [`../../skills/README.md`](../../skills/README.md).

## Four-Step Pipeline

[`../run_eval.py`](../run_eval.py) serves as the CLI entry point and backward-compatible re-export
facade over four modules:

1. [`schema_lint.py`](schema_lint.py) — static `SKILL.md` frontmatter and `EVAL.yaml` schema,
   tool-allowlist, and three-tier scorer resolution checks.
2. [`live_backend.py`](live_backend.py) and [`shims/`](shims/) — isolated `git worktree` execution,
   command wrapper installation, deterministic workspace and trace checks
   (`no_tracked_template_edits`, `skill_triggering`, `no_tf_read`, `no_generated_file_edits`,
   `ai_assisted_tag`, `blueprint_reexpand`, `script_validators`), and read-only Gemini judge grading.
3. [`plan_parser.py`](plan_parser.py) — offline self-consistency backend (`MockEvaluatorBackend`)
   and `[PROPOSED REMEDIATION PLAN]` confirmation and bypass checks (`verify_assertions`).
4. [`reporting.py`](reporting.py) — `--changed-from` git-diff skill selection
   (`resolve_changed_skills`), parallel case orchestration (`evaluate_skill`), and JSON/Markdown
   scorecard rendering.

## How `shims/` Gatekeepers Work

During `--live` runs, `LiveCLIEvaluatorBackend` installs six Bash wrappers from [`shims/`](shims/)
across `<wt>/{gcluster,ghpc}` and `<wt>/.live-shims/{gcluster,ghpc,terraform,make,kubectl,gcloud,gsutil}`
(prepending only `.live-shims` to `PATH`):

- **Forward local read-only `gcluster` commands**: `explain`, `expand`, and `create` delegate to
  `.live-shims/gcluster.real` (lazily built via `make` when needed).
- **Return offline notices on read-only cluster/cloud probes**: Allowlisted read-only `kubectl`
  (`get`, `describe`, `logs`), `gcloud`, `gsutil`, and live `gcluster cluster|job` inspection
  subcommands write offline stderr notices and exit `0` without making live network or cloud API calls.
- **Fail closed on mutating commands**: State-changing commands (`gcluster deploy|destroy`,
  `gcluster job submit|cancel`, mutating `kubectl`/`gcloud`/`gsutil`, non-build `make` flags/targets,
  and `terraform` other than `version`/`--help`) append the command to `$CTK_SHIM_AUDIT_DIR/blocked.log`
  and exit `1`, failing `safety_killswitch`.

## Known Maintenance Hotspots

- **Markdown plan parsing ([`plan_parser.py`](plan_parser.py))**: When `expect_blocked_action: true`
  is set, destructive commands listed in `forbidden_commands` are permitted inside
  `[PROPOSED REMEDIATION PLAN]`'s `Proposed Action` block (fenced or bulleted) while remaining
  forbidden everywhere else, and bypass flags (`--auto-approve`, `-l IGNORE`, `--skip-validators`)
  are always rejected. Keep nested fence and indentation edge cases covered in
  `test_remediation_plan_parser_and_bypass_detection`.
- **CLI flag skip-lists ([`shims/`](shims/))**: `kubectl.sh`, `gcloud.sh`, `gsutil.sh`, and
  `gcluster.sh` scan past global value-taking flags (such as `-n`, `--project`, `--format`, and `-d`)
  to locate the active verb and separate `exit 0` read-only queries from `exit 1` mutating verbs.
  Update those skip-lists alongside `test_default_deny_shims_and_flag_value_guards` when adding flags.
- **Gemini CLI session traces ([`live_backend.py`](live_backend.py))**: Because `gemini -o json`
  stdout omits per-tool call arguments, Turn-1 `SKILL.md` read checks and `.tf` path/edit attribution
  inspect matching session files under `~/.gemini/tmp/.../chats/*.json(l)`. Re-verify session trace
  structure whenever bumping the pinned `@google/gemini-cli` version (`0.68.0`) in Cloud Build.
