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

9. **Error Handling / String Matching:**
   * **Guideline:** When using string matching to categorize errors, place specific error patterns before generic patterns.
   * **Rationale:** Because string matching often stops at the first match, a specific error wrapped in a generic message will be incorrectly categorized if generic patterns are matched first.

By focusing on these areas, you can help maintain the quality and consistency of the Cluster Toolkit codebase.

# Cluster Toolkit Code Review Guidelines

## Infrastructure as Code (Terraform & Ansible)

**Guideline**: Do not embed complex inline shell scripts directly within YAML configurations (e.g., Ansible playbooks, Kubernetes manifests). Extract them into separate script files.
**Rationale**: Separate scripts improve readability, maintainability, and allow for proper error handling and exit codes that are difficult to manage within YAML blocks.

**Guideline**: Pass external variables to Ansible shell tasks via the `environment` keyword rather than directly interpolating them via Jinja templates (e.g., `{{ var }}`). Prefer `ansible.builtin.command` over `ansible.builtin.shell` when shell-specific features are not required.
**Rationale**: Direct interpolation can lead to command injection or syntax errors. `command` is more secure and efficient for simple executions.

**Guideline**: Place resource-level precondition validations in the `lifecycle` block of a resource that is always created, rather than in conditionally created child resources.
**Rationale**: If a precondition is attached to a conditionally created resource (e.g., `count = 0`), the validation is bypassed when the resource is not created, potentially allowing invalid configurations.

**Guideline**: Use `lifecycle` `precondition` blocks to validate feature dependencies in Terraform. Fail explicitly with a clear error message instead of silently overriding user settings.
**Rationale**: Failing explicitly prevents deploying invalid configurations and avoids the confusion of silently changing user-provided configurations.

**Guideline**: Use `one(...)` instead of indexing `[0]` when referencing resources created with `count` that could evaluate to 0 or 1.
**Rationale**: Indexing with `[0]` on a resource that was conditionally created with `count = 0` causes a plan-time crash due to an out-of-bounds index.

**Guideline**: Ensure explicit module dependencies between related Kubernetes controllers using `depends_on`.
**Rationale**: Deploying interdependent controllers in parallel without explicit dependencies can cause crash loops if required CRDs are not yet registered with the Kubernetes API server.

**Guideline**: Initialize `target_size` to 0 and add it to `ignore_changes` when deploying GCP Stateful Managed Instance Groups (MIGs) that bind pre-allocated static IPs to specific names. Set `max_unavailable_fixed` to at least the number of zones when using regional MIGs with the `RECREATE` method.
**Rationale**: Avoids doubling instances and costs due to GCP Provider declarative ordering limitations. For regional MIGs, the GCP API requires `max_unavailable` to not be less than the number of zones for `RECREATE` replacements.

**Guideline**: Refactor duplicate Ansible verification tasks (like CMEK validation) into common, reusable task files. It is acceptable to use string matching on `stdout` in Ansible for complex Bash blocks instead of breaking them up.
**Rationale**: Reduces maintenance overhead, prevents logic drift, and avoids excessive log clutter compared to breaking scripts into many small `ansible.builtin.*` tasks.

## Blueprint & Module Design

**Guideline**: Do not hardcode internal infrastructure details (GCP project IDs, buckets, zones, regions, service accounts, reservation names) as fallback defaults or in example blueprints. Use variables (e.g., `$(vars.zone)`), empty strings, or generic placeholders instead.
**Rationale**: Hardcoding specific internal resources leaks configuration to the public, makes blueprints non-reusable, and causes deployment/validation failures for other users.

**Guideline**: Define configurable settings like network IP ranges and prefix lengths in the top-level `vars` block rather than hardcoding them inside module settings. Do not explicitly set or pass `labels` variables in blueprint module configurations.
**Rationale**: Referencing top-level variables keeps the configuration clean and customizable. Cluster Toolkit automatically injects the global `labels` variable, so redefining it causes redundancy and compilation errors.

**Guideline**: Export configuration state variables (like `enable_confidential_nodes`) as outputs from core modules so that downstream modules can dynamically inherit them.
**Rationale**: Preserves module flexibility for hybrid setups and avoids adding hard validation constraints, allowing child modules to stay in sync automatically.

## Go Programming & Testing

**Guideline**: Avoid mutating or relying on global state. Use local variables, pass arguments explicitly for validation, and keep package-level variables unexported. In unit tests, explicitly reset all global variables between test cases.
**Rationale**: Global state makes commands non-reentrant, causes test pollution, and can lead to unexpected validation bypasses.

**Guideline**: Avoid iterating over maps (especially in Go templates or when generating manifests). Sort the map keys and iterate over a static slice instead. Do not mutate a map while concurrently iterating over it.
**Rationale**: Go map iteration order is randomized. Relying on it can lead to non-deterministic behavior, flaky tests, and unexpected diffs in generated outputs like GitOps manifests.

**Guideline**: Register `defer recover()` blocks at the very beginning of a function to ensure all potential panics are caught. Do not swallow panics or errors silently; return and surface the actual errors.
**Rationale**: Placing the recovery defer too late leaves early code paths unprotected. Swallowing errors obscures underlying issues, whereas returning them improves debugging and CLI usability.

**Guideline**: Structure table-driven test cases to verify the entire expected output (e.g., a full map) rather than using fragmented `wantKey`/`wantValue` combinations. Isolate configuration directories in tests using `t.TempDir()` and `t.Setenv()`.
**Rationale**: Comparing the full expected object makes assertions explicit and idiomatic. Isolating config directories prevents modifying a developer's local configuration or causing flaky tests in CI/CD.

**Guideline**: Unmarshal JSON into a temporary struct copy rather than modifying a global or shared struct directly.
**Rationale**: If unmarshaling fails due to malformed JSON, the target struct can be left in a corrupted state. A temporary copy preserves the original state on failure.

**Guideline**: Group related CLI commands under appropriate subcommands and output raw values instead of decorated strings when appropriate.
**Rationale**: Keeps the CLI namespace clean. Outputting raw values makes the CLI output scriptable and easily pipeable.

**Guideline**: Convert raw error strings and hardware strings (like accelerator labels) to lowercase before evaluating them against substring matchers. Order error matchers from most specific to most generic.
**Rationale**: Standard errors and upstream APIs may use varying capitalizations. Specific-to-generic ordering prevents specific errors wrapped in generic messages from being incorrectly categorized.

**Guideline**: Use established SemVer libraries for version comparison rather than manual string parsing (e.g., `Sscanf`).
**Rationale**: Manual parsing fails on abbreviated versions and doesn't handle pre-release precedence logic according to SemVer standards.

**Guideline**: Remove redundant validation checks in downstream helper functions if the condition is already guaranteed by the caller. Flatten nested logic and use early returns to improve readability.
**Rationale**: Redundant checks add dead code. Deeply nested if-else blocks are harder to read, and early returns reduce cognitive load.

## Shell Scripting

**Guideline**: Implement "fail-fast" mechanisms in polling loops, and ensure post-deployment verification scripts or jobs exit with a non-zero status code on failure (e.g., using `|| { echo "..."; exit 1; }`). Enable `set -o pipefail` in scripts using pipelines.
**Rationale**: Failing fast conserves CI resources. Exiting with a proper status code ensures that the shell doesn't silently exit with 0, which would mask hardware/security faults in environments like GKE.

**Guideline**: When using a `SIGTERM` trap to handle preemption for a user-provided script, wrap the command in a subshell block `( ... ) & PID=$!`.
**Rationale**: Ensures the entire script runs as a single background job, allowing the trap to capture the correct PID, unlike running multi-line commands directly in the background.

**Guideline**: When updating critical system files (like `/etc/hosts`), write to a temporary file first and use an atomic replace operation (e.g., `mv`).
**Rationale**: Writing directly is not atomic and can corrupt the file if the operation is interrupted midway.

**Guideline**: Use `hostname | cut -d. -f1` instead of `hostname -s` in startup scripts to extract the short hostname safely.
**Rationale**: `hostname -s` may attempt a network-based DNS lookup, which can hang or fail during early boot if DNS is not fully initialized.

## Python & General Programming

**Guideline**: Always guard against null or `None` values when accessing attributes of optional configuration objects. Implement fully recursive conversion methods when wrapping nested data structures.
**Rationale**: Prevents runtime `AttributeError`s when features are omitted, and avoids shallow conversion bugs in deeply nested structures.

**Guideline**: When overriding `__getattr__` in custom classes, ensure dunder attributes explicitly raise `AttributeError`.
**Rationale**: Standard library functions like `copy.deepcopy()` or `pickle` rely on `AttributeError` for dunder attributes to identify supported behaviors.

**Guideline**: When replacing base image paths in container configurations, trim the base image prefix and append the remaining suffix rather than splitting on colons (`:`).
**Rationale**: Robustly handles both image tags (using `:`) and image digests (using `@sha256:`).

## Kubernetes & Google Cloud Specifics

**Guideline**: For Pathways coordinator pods, use a hybrid scheduling model: specify explicit, low resource requests (e.g., 2 CPU, 8Gi memory) and high, static resource limits (e.g., 24 CPU, 100Gi memory).
**Rationale**: High limits prevent OOM kills during heavy compilation, while low requests ensure scheduling flexibility on head nodes.

**Guideline**: When deriving or formatting the Pathways platform string from GKE labels, ensure TPU v5p architectures explicitly map to `tpuv5`, and do not strip the `tpu` prefix (e.g., use `tpuv5e`).
**Rationale**: The Pathways server strictly validates platform string arguments; bypassing mapping logic or omitting prefixes causes container startup crashes.

**Guideline**: When checking permissions for a resource using `kubectl auth can-i`, use the fully qualified resource name instead of the `--group` flag.
**Rationale**: The `--group` flag is for user group impersonation, not for specifying the API group of the resource, which causes false negatives.

**Guideline**: Use valid, stable CUDA images (e.g., `nvidia/cuda:12.6.0-base-ubuntu22.04`) in GKE configurations and do not mount the shared `/tmp` volume in the `pathways-rm` container.
**Rationale**: Using non-existent images causes `ImagePullBackOff`. Mounting the shared temp directory in the RM container is redundant.

**Guideline**: When validating node auto-provisioning availability or computing specific and generic max limits on GKE, check against both specific limit keys and generic GPU/TPU fallback keys. Process limits in two distinct passes instead of one.
**Rationale**: Prevents false negatives if only generic limits are configured, and avoids loop-order dependency bugs where generic limits might incorrectly overwrite specific ones.

**Guideline**: When setting `omit_external_ip: true` for a Packer image build, ensure the builder VM has an alternative way to access the internet (like a Cloud NAT module).
**Rationale**: The VM needs internet access to download packages; without it, the build will fail.

## Maintenance, Documentation & Dependencies

**Guideline**: Keep documentation, inline comments, error messages, and example indices (e.g., `examples/README.md`) in sync with code changes. When deprecating features, refactoring parameters, or changing default values, update the corresponding docs. Use direct links to `README.md` files rather than parent directories.
**Rationale**: Misleading or outdated comments cause configuration parsing failures and confuse users. Keeping examples registered maintains repository hygiene.

**Guideline**: Always upgrade build-time tools (pip, setuptools, wheel) in a separate step before installing other packages via pip. Ensure vulnerable package versions in dependency files are upgraded.
**Rationale**: Prevents build failures caused by older build tools and mitigates security risks.

**Guideline**: When updating tool versions in dependency scripts, ensure the corresponding checksums file is regenerated. When updating default image OS families, ensure legacy monitoring agents are replaced by newer supported agents.
**Rationale**: Ensures CLI tools use expected binaries, and prevents integration test failures due to unsupported legacy agents on modern operating systems.

## UI & Frontend (Accessibility and JS)

**Guideline**: Provide empty `alt=""` attributes for decorative/redundant images. For interactive image links, use `alt` text that describes the action/destination. Avoid words like 'logo' or 'image' in `alt` text.
**Rationale**: Improves screen reader experience by avoiding redundant announcements and correctly describing link purposes.

**Guideline**: Add null checks when interacting with DOM elements that are conditionally rendered.
**Rationale**: Prevents 'TypeError' when accessing elements excluded by server-side templating.

**Guideline**: Use recursive `setTimeout` instead of `setInterval` for AJAX polling logic, and ensure polling stops on all terminal states (success, error, deleted).
**Rationale**: Prevents overlapping requests, network congestion, and infinite background requests that degrade performance. Use a transparent 1x1 pixel GIF data URI for placeholders instead of an empty `src=""` to prevent unnecessary browser requests.

