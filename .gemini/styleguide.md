# Cluster Toolkit - Code Review Style Guide for Gemini

When reviewing Pull Requests for the Google Cloud Cluster Toolkit, please adopt the persona of an expert Software Engineer. Your primary focus should be on ensuring changes enhance the project's long-term health. Prioritize the following:

* **Technical Excellence:** Ensure code is well-structured, efficient, and follows best practices.
* **Maintainability:** Code should be easy to understand, modify, and extend.
* **Testing:** Changes must be well-tested. Encourage comprehensive unit and integration tests.
* **Documentation:** Ensure documentation is updated, including in-code comments, module READMEs, and index files.

Pay close attention to the following specifics:

1. **Blueprint Authoring (YAML):**
   * Ensure the `use` block is preferred for module dependencies within blueprints. Explicit variable linking (e.g., `setting = $(module.output)`) should only be used when necessary to resolve ambiguity.
   * Verify that module sources are correct and the referenced modules exist.
   * Check for logical grouping of modules within `deployment_groups`.
   * Ensure variable usage is correct (e.g., `$(vars.name)`, `$(module.id.output)`).
   * Validate the overall structure and syntax of the YAML blueprint.

2. **Terraform Module Development (HCL):**
   * Verify module inputs and outputs are consistent and well-defined.
   * Check for clear variable definitions in `variables.tf` with descriptions, types, and sensible defaults where applicable.
   * Ensure resources within the module are logically structured.
   * Encourage the use of best practices for writing clean and maintainable Terraform code.
   * Ensure new modules are placed in the correct directory (`modules/` or `community/modules/`) and within the appropriate subdirectory (e.g., `compute`, `network`, `file-system`, `scheduler`, etc.).

3. **Go Language:**
   * Follow standard Go idioms and best practices (e.g., error handling, naming).
   * Ensure code is well-commented, especially public functions and complex logic.
   * Check for test coverage for new or modified Go code.

4. **Documentation:**
   * **CRITICAL:** If new modules (core or community) are added, ensure they are added to the index in `modules/README.md`.
   * **CRITICAL:** If new examples (core or community) are added, ensure they are added to the index in `examples/README.md`.
   * In-code comments should be clear and explain the *why* not just the *what*.
   * Module `README.md` files should be clear and provide sufficient information on usage, inputs, and outputs.

5. **Testing:**
   * New features or bug fixes should ideally be accompanied by tests.
   * Tests should be clear and cover both happy paths and edge cases.
   * Encourage the use of the existing testing frameworks and patterns within the project.

6. **PR Description:**
   * The PR description should clearly explain the purpose of the change and the problem it solves.
   * It should mention how the changes were tested.

7. **Structure:**
   * Confirm adherence to the project structure (e.g., core vs. community).

8. **Temporal Context:**
   * The current year is 2026.
   * When reviewing copyright headers, acknowledge that 2026 is the correct current year.
   * Do not suggest changing "2026" to "2025" or any other year.

By focusing on these areas, you can help maintain the quality and consistency of the Cluster Toolkit codebase.

---
name: ctk-go-sage
description: Go coding standards and performance guidelines for Cluster Toolkit.
generated_by: sage_framework
---

# Cluster Toolkit Guidelines: Go Coding Standards (Concurrency, Safety, Performance)

*   **Guideline**: Write thread-safe Go code by synchronizing access to shared package-level state and global variables (e.g., using `sync.RWMutex`). Never execute external commands, file I/O, or network requests while holding a lock. Initialize shared clients using `sync.Once`.
    *   **Rationale**: Synchronous or high-latency network/external calls inside a lock block other threads, leading to deadlock risks or severe performance degradation. Thread-safe initialization avoids duplicate clients and memory corruption (PR 5534, PR 5703, PR 5547, PR 5426).

*   **Guideline**: Do not run network calls, file I/O, or CPU-intensive setup during package initialization (`init()` functions or global variables). Instead, load resources lazily or defer them to CLI runner hooks (e.g., Cobra `PreRun`).
    *   **Rationale**: Package-level initialization runs eagerly on startup. Blocking startup with network/I/O requests introduces lag for every CLI command (even offline ones) and causes hangs in offline environments (PR 5561, PR 5539, PR 5589).

*   **Guideline**: Implement robust network client and resource lifecycle management:
    *   Always verify the response error is nil before deferring `Body.Close()`.
    *   Validate HTTP status codes before parsing the response body.
    *   Enforce context-based timeouts on all external network requests.
    *   Reuse expensive API or database clients across savings rather than recreation.
    *   **Rationale**: Prevents nil-pointer dereferences (closing a nil body), resource leaks (unclosed sockets), and application hangs when remote endpoints are slow or unresponsive (PR 5589, PR 5547, PR 5553, PR 5519, PR 5478, PR 5431).

*   **Guideline**: Optimize CPU and memory performance in Go code:
    *   Pre-allocate slices when the target capacity is known using `make([]T, length)`.
    *   Define and compile regular expressions (e.g., using `regexp.MustCompile`) and static maps as package-level variables rather than initializing them repeatedly inside functions or loops.
    *   Use `strings.Contains` instead of regular expressions for simple substring matches.
    *   Avoid spawning external processes (e.g., `gcloud`) for cheap checks; check environment variables or local files first.
    *   Short-circuit loops with `break` once a search target is matched.
    *   Use `filepath.WalkDir` (which avoids calling `os.Lstat` on every file) instead of `filepath.Walk`.
    *   Use a bounded worker pool for concurrent processing when executing multiple remote requests.
    *   **Rationale**: Minimizes slice allocations, avoids repeated regex compilation overhead, limits slow subprocess creation, and scales well when handling large numbers of resources (PR 5534, PR 5523, PR 5468, PR 5422, PR 5590, PR 5494, PR 5354, PR 5431, PR 5553).

*   **Guideline**: Write defensive and panic-safe Go code:
    *   Perform nil checks on structures returned by helper functions or external libraries before field access.
    *   Verify slice lengths (`len(slice) > 0`) instead of only nil-checking before index access.
    *   Validate the output of string conversions (e.g., checking for errors and preventing division by zero).
    *   Lazily initialize struct maps/caches if they are not guaranteed by a constructor.
    *   Support graceful degradation: return neutral values (like `nil, nil`) for unsupported formats in utility readers.
    *   Avoid embedding test-specific flags or checks in production code; use environment variables or interfaces.
    *   Parse config files robustly: strip comments, trim whitespaces, and avoid fragile prefix matching (e.g. `strings.HasPrefix`).
    *   **Rationale**: Prevents common nil-pointer panics, index-out-of-bounds panics, and makes the system resilient to malformed configuration or missing fields (PR 5482, PR 5730, PR 5455, PR 5590, PR 5426, PR 5703, PR 5502).

*   **Guideline**: Adhere to standard Go styling, structuring, and path handling:
    *   Follow Google Go Style: group standard library imports separately from third-party ones, use CamelCase naming, and keep package-level variables unexported if unused outside.
    *   Embed static assets and templates into the binary using `embed.FS` to prevent relative path resolution failures when the binary runs from different directories.
    *   Use the `path` package instead of `path/filepath` for forward-slash paths (e.g., URLs, APIs) to prevent Windows backslash conversion. Use `filepath.ToSlash` to normalize OS-level paths on Windows.
    *   Consolidate struct configurations: pass configurations to constructors and store them in struct fields, reuse existing structs, and prefer copying shared maps/configs over in-place mutation to avoid side-effects.
    *   **Rationale**: Ensures code readability, prevents platform-specific issues (e.g., path separator differences on Windows vs Linux), and prevents side-effects from mutating shared state (PR 5519, PR 5494, PR 5523, PR 5431, PR 5553, PR 5482, PR 5598, PR 5420).
---
name: ctk-shell-sage
description: Bash and Ansible automation scripting guidelines for Cluster Toolkit.
generated_by: sage_framework
---

# Cluster Toolkit Guidelines: Shell Scripting & Automation (Bash, Ansible)

*   **Guideline**: Follow robust coding standards in Ansible playbooks:
    *   Always apply the `default` filter to optional variables in tasks (e.g., `{{ var | default('') }}`) to avoid undefined variable rendering failures.
    *   Use explicit boolean comparisons in `when` conditionals (e.g., `when: my_var | bool`) instead of relying on implicit truthy evaluations.
    *   Quote all shell variables (e.g., `"${MY_VAR}"`) within playbook `shell` tasks to avoid word splitting and injection risks.
    *   Keep playbook files clean by removing commented-out tasks and configurations.
    *   **Rationale**: Ansible evaluates all variables even in skipped paths, which causes runtime errors for undefined vars. Explicit boolean checks are required for modern Ansible cores, and quoting prevents shell parsing errors and security vulnerabilities (PR 5710, PR 5400, PR 5742, PR 5650, PR 5528).

*   **Guideline**: Handle temporary files and directory paths securely:
    *   Avoid predictable paths in public directories (like `/tmp/something`). Always use `mktemp -d` or `ansible.builtin.tempfile` to create random, unique directories.
    *   Set restrictive permissions (e.g., `mode: '0700'` or `chmod 700`) on temporary paths.
    *   Avoid running temporary directory setup with `sudo` in public spaces.
    *   In multi-user or shared compute node environments (e.g., Slurm clusters), append the user identifier (`${UID}`) or username to shared runtime directories, container runtime caches (e.g., Enroot/Pyxis), and build caches.
    *   **Rationale**: Predictable paths in public directories are vulnerable to symlink attacks. Sharing paths without unique identifiers causes permission errors when different users attempt to execute tasks on the same shared host (PR 5335, PR 5535, PR 5308, PR 5504).

*   **Guideline**: Write robust and compatible shell scripts and automation commands:
    *   Use backticks (\`...\`) for command substitution in shell tasks embedded inside blueprint YAMLs to prevent conflicts with the toolkit's `$(vars.x)` interpolation.
    *   Enforce explicit error checking (`set -e` or check return codes) on all subprocess calls in runner scripts.
    *   Ensure startup scripts are idempotent by checking conditions (e.g., `mountpoint -q /mnt` or checking user presence) before acting, so they run safely on reboot.
    *   Keep timeout calculations absolute rather than resetting them between script execution stages.
    *   Comply with modern OS packaging rules (PEP 668): use virtual environments (`venv`) for Python packages and avoid deprecated `apt-key` commands.
    *   **Rationale**: Prevents syntax clashes with toolkit interpolation, ensures scripts fail-fast on sub-command failures, allows node reboots without startup crashes, and aligns with modern OS security rules (PR 5456, PR 5593, PR 5695, PR 5400).
---
name: ctk-reviewer
description: Sage coding guidelines and review best practices for the Google Cloud Cluster Toolkit.
---

# Google Cloud Cluster Toolkit Sage Reviewer Guidelines

This skill contains coding guidelines and review best practices for the Google Cloud Cluster Toolkit, synthesized from historical PR reviews and split by theme.

## Installation

To enable this skill globally in Jetski, link this directory to your Jetski skills customization root:

```bash
mkdir -p ~/.gemini/jetski/skills && ln -sf /google/src/head/depot/google3/experimental/users/sarthakag/ctk-reviewer/ ~/.gemini/jetski/skills/ctk-reviewer
```

## How to Use

When this skill is active, the agent must check the target technologies of the CL or PR being reviewed/edited and read the detailed guidelines from this directory using the `view_file` tool:

*   **Terraform & HCL**: Read [terraform.md](file:///google/src/head/depot/google3/experimental/users/sarthakag/ctk-reviewer/terraform.md)
*   **Go Coding Standards**: Read [go.md](file:///google/src/head/depot/google3/experimental/users/sarthakag/ctk-reviewer/go.md)
*   **Shell Scripting & Ansible**: Read [shell.md](file:///google/src/head/depot/google3/experimental/users/sarthakag/ctk-reviewer/shell.md)
*   **Testing & Verification**: Read [testing.md](file:///google/src/head/depot/google3/experimental/users/sarthakag/ctk-reviewer/testing.md)
*   **Error Handling & Telemetry**: Read [telemetry.md](file:///google/src/head/depot/google3/experimental/users/sarthakag/ctk-reviewer/telemetry.md)
*   **Validation & Constraints**: Read [validation.md](file:///google/src/head/depot/google3/experimental/users/sarthakag/ctk-reviewer/validation.md)
---
name: ctk-telemetry-sage
description: Error handling and background telemetry guidelines for Cluster Toolkit.
generated_by: sage_framework
---

# Cluster Toolkit Guidelines: Error Handling & Telemetry

*   **Guideline**: Design non-intrusive, secure, and resilient telemetry:
    *   Telemetry operations must fail silently, returning empty strings `""` for missing or failed metrics rather than throwing visible console errors or writing placeholders like "Unknown".
    *   Exit hooks for telemetry upload must run asynchronously or have short, strict timeouts to prevent CLI exit delays.
    *   Never log telemetry failures as critical or fatal errors, and avoid using fatal log calls for user-driven clean stops.
    *   Explicitly whitelist allowed metadata keys when exporting deployment configurations to telemetry servers; do not serialize raw variable maps or environment blocks to avoid leaking local credentials, IAM keys, or secrets.
    *   Deduplicate collected metadata arrays (e.g., node architectures, regions) before aggregating the telemetry payload.
    *   **Rationale**: Telemetry is background execution data. It must never block the user, pollute CLI stdout/stderr, or expose private deployment credentials in aggregated logs (PR 5502, PR 5519, PR 5518, PR 5704, PR 5478, PR 5494).

*   **Guideline**: Implement descriptive, robust, and clean logging/error handling:
    *   Contextualize error messages by including path names or resource identifiers.
    *   Perform case-insensitive string checks when matching subprocess errors to handle variance in system outputs.
    *   Use polling retry loops with a minimum bound of 1 iteration (e.g., `max(1, timeout/snooze)`) when waiting for files generated asynchronously by parallel nodes.
    *   Wrap flaky start commands in try-except blocks, but verify the final service status strictly to ensure failures are caught.
    *   Silence verbose sub-commands (like extracting zip archives in build steps) and output clean summary lines to keep build logs readable.
    *   **Rationale**: Diagnostic clarity is enhanced by having source references in logs. Polling checks prevent race failures on startup, and clean build logs make actual failures easier to identify (PR 5482, PR 5717, PR 5352, PR 5759, PR 5710).
---
name: ctk-terraform-sage
description: Terraform and HCL coding guidelines for Cluster Toolkit.
generated_by: sage_framework
---

# Cluster Toolkit Guidelines: Infrastructure as Code (Terraform/HCL)

*   **Guideline**: Use the `any` type for blueprint variables representing complex structures (like maps or objects) to allow users to input native HCL/YAML blocks. To maintain backward compatibility with older configurations that pass JSON strings, use a dynamic fallback pattern: `try(jsondecode(var.my_var), var.my_var)`.
    *   **Rationale**: Direct HCL blocks improve usability and reduce syntax errors compared to escaped JSON strings. The dynamic fallback ensures the parser handles both formats seamlessly without type coercion failures (PR 5514).

*   **Guideline**: Implement validation constraints using Terraform `validation` and `precondition` blocks within modules to catch configuration errors early:
    *   Restrict string variables to allowed values (using `contains`).
    *   Enforce co-dependency rules (e.g., ensuring secondary parameters like dynamic node pools are only active when their primary enablers are set).
    *   Validate external version compatibilities (e.g., GKE version requirements).
    *   Enforce structural constraints (e.g., naming length limits for GCP resources).
    *   Validate format structures (e.g., verifying GCP Resource IDs match the expected path pattern: `projects/<project>/locations/<location>/...`).
    *   **Rationale**: Validating variables during `terraform plan` avoids mid-deployment failures and provides clean, actionable error messages instead of raw provider API errors (PR 5569, PR 5352, PR 5375, PR 5438, PR 5543, PR 5628, PR 5452, PR 5562).

*   **Guideline**: Align Terraform variable names with standard naming schemes of the underlying resources or blocks they control, and follow standard toolkit conventions (e.g., `deployment_name`, `region`, `network_self_link`).
    *   **Rationale**: Consistency with underlying cloud provider APIs reduces cognitive load and makes module configurations self-documenting, while standard naming conventions enable automatic variable inference in the toolkit (PR 5600, PR 5731, PR 5569).

*   **Guideline**: Manage Terraform dependencies and provider versions carefully:
    *   Declare required GCP services in `metadata.yaml` rather than using inline `google_project_service` resources inside modules.
    *   Remove unused providers from `providers.tf` and `versions.tf`.
    *   Prefer native or widely supported providers over third-party providers.
    *   Update minimum provider version constraints in `versions.tf` and `README.md` when introducing features that depend on recent provider updates.
    *   Pin the required Terraform version to the specific officially supported version.
    *   **Rationale**: Minimizes configuration bloat, avoids provider inheritance conflicts, ensures compatibility with GCP features, and prevents deployment failures (PR 5562, PR 5592, PR 5731, PR 5537, PR 5375, PR 5485).

*   **Guideline**: Ensure security-by-default and trackability in all resource definitions:
    *   Default `deletion_protection` to `true` to prevent accidental resource loss.
    *   Default `allow_unauthenticated` to `false` for new services.
    *   Keep `enable_external_dns_endpoint` default as `false` for GKE control planes to avoid public internet exposure.
    *   Expose an optional `labels` variable (type `map(string)`) and merge it with module-specific labels to enable billing and tracking.
    *   **Rationale**: Protects production environments from accidental data loss, minimizes the attack surface, and ensures proper resource classification (PR 5567, PR 5336, PR 5559).

*   **Guideline**: Follow clean variable declaration and documentation standards:
    *   Provide defaults (like `[]` or `null`) for optional configuration variables.
    *   Document default values and rationale in the variable's description.
    *   Specify the expected format of variables (e.g., resource ID path) with examples.
    *   Use `set(string)` for variables used in `for_each` resource loops to guarantee uniqueness.
    *   Avoid inline complex expressions in resources; refactor them into local variables.
    *   Ensure `README.md` is updated (regenerating inputs/outputs) when changing variables.
    *   **Rationale**: Improves code readability, usability, and prevents configuration failures due to duplicate members in loops or malformed resource references (PR 5595, PR 5569, PR 5379, PR 5574, PR 5564, PR 5476).

*   **Guideline**: Design platform configurations for durability:
    *   Disable advanced or disruptive platform features (e.g., autoscalers that mutate state dynamically) by default to prevent unexpected runtime changes on upgrades.
    *   Modernize default hardware choices (e.g., choosing newer machine generations) for new setups while preserving backward compatibility.
    *   Avoid using strict enum validation lists for rapidly evolving cloud provider properties (e.g., addon components or monitoring options) to prevent blocking future updates.
    *   **Rationale**: Maintains backward compatibility while ensuring the setup uses modern, performant defaults and doesn't get blocked by rigid validation configurations as cloud provider APIs evolve (PR 5610, PR 5370, PR 5722).

*   **Guideline**: Handle Kubernetes manifest templating and merging cleanly:
    *   When merging configurations (e.g. ClusterQueues), merge `metadata` and `spec` blocks separately (deep merge) rather than doing a shallow merge on the root block.
    *   Do not programmatically override scheduling properties if the templates themselves manage them.
    *   Auto-wire feature flags by exposing them as module outputs from provisioning modules and inputting them into consumption modules.
    *   **Rationale**: Prevents dropping critical user-defined spec attributes or metadata annotations when combining templates, reduces manual configuration overhead, and keeps deployment templates clean (PR 5645, PR 5655).
---
name: ctk-testing-sage
description: Integration and unit testing guidelines for Cluster Toolkit.
generated_by: sage_framework
---

# Cluster Toolkit Guidelines: Testing & Verification

*   **Guideline**: Design robust, isolated, and readable unit tests:
    *   Use table-driven tests to verify multiple input shapes and error handling paths.
    *   Ensure validation and sanitization tests check edge cases (empty values, capitalization, special symbols, length constraints).
    *   Abstract system commands and external APIs behind interfaces or package-level mock hooks to keep tests offline and hermetic.
    *   Avoid raw JSON string assertions; parse output into structs and verify fields.
    *   Isolate tests from the host environment: use `t.Setenv` (which handles automatic cleanup) and explicitly set required environment variables to clean states.
    *   Always restore any modified global state (e.g., default HTTP transport or package-level Cobra CLI flag variables) after test execution (e.g., using `t.Cleanup` or `defer` blocks) to prevent test contamination and flakiness.
    *   Maintain project-wide test coverage standards (e.g., 80% minimum for new packages).
    *   **Rationale**: Hermetic, isolated tests reduce flakiness, improve speed, prevent environment leakage, and ensure changes do not contaminate parallel test executions (PR 5489, PR 5468, PR 5590, PR 5518, PR 5490, PR 5547, PR 5354).

*   **Guideline**: Ensure robust integration test setup and configuration hygiene:
    *   Use dynamic zone selection (e.g., `find_available_zone.sh`) to locate available GCP zones and prevent quota or resource exhaustion failures during test runs.
    *   Prefer static, reusable VPC networks for integration tests (with proper locking) over creating dynamic networks per-build, to avoid hitting cloud provider service limits (e.g., Filestore peered network limits).
    *   Revert all local overrides, fork references, and custom branch configs in CI/CD playbooks/scripts before submitting.
    *   Keep integration test runner configurations and playbooks synced with blueprint modifications to prevent target resource mismatch.
    *   Consolidate duplicated integration verification checks into reusable shared playbook tasks (using `include_tasks`).
    *   **Rationale**: Prevents flakiness caused by resource limits, ensures clean and production-ready CI/CD scripts, and reduces cleanup drift (PR 5591, PR 5315, PR 5308, PR 5544, PR 5757).

*   **Guideline**: Follow reliable Kubernetes testing and validation patterns:
    *   Always specify `--all-containers` or an explicit container name in `kubectl logs` commands to avoid errors when pods contain sidecars (like GCS Fuse).
    *   Use unique resource names for test jobs in shared namespaces to prevent namespace collision and delete races.
    *   Keep basic sanity tests (like GPU availability checks) active when appending new subsystem tests (like GCS mount checks).
    *   **Rationale**: Kubernetes rejects simple logs requests on multi-container pods. Unique naming avoids flaky collision errors, and keeping baseline checks prevents hardware-level regressions (PR 5742).
---
name: ctk-validation-sage
description: TPU and mutual exclusion validation guidelines for Cluster Toolkit.
generated_by: sage_framework
---

# Cluster Toolkit Guidelines: Validation & Constraints

*   **Guideline**: Apply strict, centralized, and rule-based validation for TPU and hardware configurations:
    *   Validate hardware capabilities at the orchestrator/resolution layer using resolved names, not at CLI entry points.
    *   Use centralized helper functions to match TPU families and normalize shorthand notations (e.g. mapping cores vs. chips).
    *   Validate TPU topologies using mathematical rules (e.g., dimension positive integers, multiples of 4, or sorted checks) rather than static lists.
    *   Derive workload node counts automatically from the hardware accelerator topology parameters rather than allowing users to specify node counts manually, preventing layout/shape conflicts during scheduling.
    *   Restrict shorthand naming conventions for newer hardware accelerator models (e.g., complex TPU architectures) in configurations; enforce explicit shape/topology declarations over ambiguous chip/core counts.
    *   Bypass generic cluster compact placement checks for hardware accelerator node pools (e.g., TPU nodes) since they are compacted natively via topology allocation.
    *   **Rationale**: Hardware shapes differ across generations. Centralized mathematical validation ensures compatibility with future layouts, prevents manual user configuration errors, and avoids false failures from unrelated platform-level checks (PR 5644, PR 5590, PR 5608, PR 5737).

*   **Guideline**: Enforce strict mutual exclusion and upgrade checks for distributed cluster workloads:
    *   Raise validation errors if a user enables both a managed cloud add-on and a custom manual deployment of the same controller (e.g., managed Kueue vs. manual Helm chart).
    *   Prevent conflicting network configurations (e.g., disallowing manual network options when automatic accelerator DRANET is enabled).
    *   Verify that default values of interdependent variables (such as service tiers and minimum disk capacities) are set compatibly.
    *   For distributed workloads (like JobSet training), enforce a draining step to 0 nodes and verify checkpoint completion before GKE node pool upgrades.
    *   **Rationale**: Double deployments cause resource clashes, incompatible variable settings lead to API errors, and rolling upgrades on active distributed training without checkpointing causes job failure (PR 5731, PR 5418, PR 5379, PR 5698).
