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

9. **OS Image Policy:**
   * Apply this section only to added or modified lines. Do not comment on unchanged or deleted lines.
   * Start each comment raised under this section with its rule ID (`[IMG-1]` or `[IMG-2]`), state the risk in one sentence, and suggest a compliant replacement.
   * **[IMG-1] No `schedmd-slurm-public` images (CRITICAL).** Images and containers in `schedmd-slurm-public` are not maintained by this project, so the toolkit does not control when OS or kernel security fixes land in them. Flag:
     * Any reference to the project ID `schedmd-slurm-public`, for example `project: schedmd-slurm-public`, `source_image_project_id: [schedmd-slurm-public]`, `projects/schedmd-slurm-public/global/images/...`, or `us-docker.pkg.dev/schedmd-slurm-public/...`.
     * Any use of `schedmd-slurm-gcp-v5-*` modules or the removed `schedmd-slurm-gcp-v6-nodeset-tpu` module, including through an older `toolkit_modules_version`; their image and container defaults are in `schedmd-slurm-public`. For TPU v5e, v5p, v6e, and TPU7x, point to the standard nodeset pattern in `examples/machine-learning/tpu-v5e-v5p-v6e/` or `examples/machine-learning/tpu7x-standard-4t/`.
     * The `schedmd-` prefix in v6 module paths (such as `community/modules/scheduler/schedmd-slurm-gcp-v6-controller`) is not itself a violation—only the `schedmd-slurm-public` project ID and the `nodeset-tpu` / `v5-*` cases above are.
     * Suggested replacement: the matching CPU or GPU image family in `advanced-compute-images` (for example, the CPU default in `community/modules/scheduler/schedmd-slurm-gcp-v6-controller/variables_controller_instance.tf` or a matching family from the [Advanced Compute Images docs](https://cloud.google.com/compute/docs/images/workload/aci-images)).
   * **[IMG-2] No dated image pins (HIGH).** An image selected by name boots the same build on every deployment and does not pick up security fixes published later to its image family. Flag a boot or source image selected by name instead of by family:
     * `name:` in `instance_image` or in any image map passed to it.
     * Packer `source_image:`; use `source_image_family:` with `source_image_project_id:` instead.
     * An image self-link without `/family/`, for example `projects/<project>/global/images/<name>`.
     * A Terraform variable default that selects an image by `name`.
     * Any of the above set through blueprint `vars`, flagging either the modified `vars` entry (for example `base_image.image`) or the modified module setting referencing it (for example `source_image: $(vars.base_image.image)`).
     * Pinned names usually contain a build date or timestamp, for example `ubuntu-2204-jammy-v20240126` or `slurm-gcp-6-12-hpc-rocky-linux-9-20260519`.
     * Not pins: family names that contain version numbers (`aci-cpu-rocky-linux-9-slurm-2605-amd64`, `ubuntu-accelerator-2404-amd64-with-nvidia-580`), Packer output settings (`image_name`, `image_family`), and container image tags.
     * If the change gives a reason for the pin, such as a driver or kernel-module compatibility constraint, still flag it, quote the reason, and ask for a linked tracking issue with a plan to return to an image family.
   * Exempt from both rules: this style guide; tests and tooling whose purpose is to detect these references; changelog, deprecation, and migration notes that describe moving away from them; reference docs explaining how to pin or roll back images (such as `docs/vm-images.md` and `docs/slurm-image-upgrade.md`); and generated files (`llms.txt`, `llms-full.txt`, `skills/cluster-blueprint-assistant/references/catalog.yaml`, `tools/validate_configs/golden_copies/`, and `terraform-docs` blocks in module `README.md` files). For generated files, comment on the source blueprint or module instead.

By focusing on these areas, you can help maintain the quality and consistency of the Cluster Toolkit codebase.
