# Blueprint Validation

The Toolkit contains "validator" functions that perform tests to ensure that deployment variables are valid and that the HPC environment can be provisioned in your Google Cloud project.

These validators run during the deployment folder creation phase (executed by `gcluster create` or the initial stage of `gcluster deploy`). This ensures that all configurations are validated strictly before any infrastructure is provisioned, and before external tools like Terraform or Packer are called.

Validation occurs at two levels:

1. **Blueprint-level Validators:** Global environment checks (e.g., project/region existence) defined in the blueprint or added implicitly.
2. **Module-level (Metadata) Validators:** Input-specific checks (e.g., regex naming patterns) defined within a module's `metadata.yaml`.

To succeed, validators often need the following services to be enabled in your blueprint project(s):

* Compute Engine API (compute.googleapis.com)
* Service Usage API (serviceusage.googleapis.com)

---

## Blueprint-level Validators

One can [explicitly define these validators](#explicit-blueprint-validators); however, the expectation is that the implicit behavior will be useful for most users. When implicit, a validator is added if all deployment variables matching its inputs are defined. Validators that have no inputs are always enabled by default
because they do not require any specific deployment variable.

Each validator is described below:

* `test_project_exists`
  * Inputs: `project_id` (string)
  * PASS: if `project_id` is an existing Google Cloud project and the active
    credentials can access it
  * FAIL: if `project_id` is not an existing Google Cloud project _or_ the
    active credentials cannot access the Google Cloud project
  * If Compute Engine API is not enabled, this validator will fail and provide
    the user with instructions for enabling it
  * Manual test: `gcloud projects describe $(vars.project_id)`
* `test_apis_enabled`
  * Inputs: none; reads whole blueprint to discover required APIs for project(s)
  * PASS: if all required services are enabled in each project
  * FAIL: if `project_id` is not an existing Google Cloud project _or_ the
    active credentials cannot access the Google Cloud project
  * If Service Usage API is not enabled, this validator will fail and provide
    the user with instructions for enabling it
  * Manual test: `gcloud services list --enabled --project $(vars.project_id)`
* `test_region_exists`
  * Inputs: `region` (string)
  * PASS: if region exists and is accessible within the project
  * FAIL: if region does not exist or is not accessible within the project
  * Typical failures involve simple typos
  * Manual test: `gcloud compute regions describe $(vars.region) --project $(vars.project_id)`
* `test_zone_exists`
  * Inputs: `zone` (string)
  * PASS: if zone exists and is accessible within the project
  * FAIL: if zone does not exist or is not accessible within the project
  * Typical failures involve simple typos
  * Manual test: `gcloud compute zones describe $(vars.zone) --project $(vars.project_id)`
* `test_zone_in_region`
  * Inputs: `zone` (string), `region` (string)
  * PASS: if zone and region exist and the zone is part of the region
  * FAIL: if either region or zone do not exist or the zone is not within the
    region
  * Common failure: changing 1 value but not the other
  * Manual test: `gcloud compute regions describe us-central1 --format="text(zones)" --project $(vars.project_id)`
* `test_module_not_used`
  * Inputs: none; reads whole blueprint
  * PASS: if all instances of use keyword pass matching variables
  * FAIL: if any instances of use keyword do not pass matching variables
* `test_deployment_variable_not_used`
  * Inputs: none; reads whole blueprint
  * PASS: if all deployment variables are automatically or explicitly used in
    blueprint
  * FAIL: if any deployment variable is unused in the blueprint

### Explicit Blueprint Validators

Validators can be overwritten and supplied with alternative input values,
however they are limited to the set of functions defined above. As an example,
the default validators added when `project_id`, `region`, and `zone` are defined
is:

```yaml
validators:
  - validator: test_module_not_used
    inputs: {}
  - validator: test_deployment_variable_not_used
    inputs: {}
  - validator: test_project_exists
    inputs:
      project_id: $(vars.project_id)
  - validator: test_apis_enabled
    inputs: {}
  - validator: test_region_exists
    inputs:
      project_id: $(vars.project_id)
      region: $(vars.region)
  - validator: test_zone_exists
    inputs:
      project_id: $(vars.project_id)
      zone: $(vars.zone)
  - validator: test_zone_in_region
    inputs:
      project_id: $(vars.project_id)
      region: $(vars.region)
      zone: $(vars.zone)
```

## Module-level (Metadata) Validators

Module-level validators are defined directly within a module's `metadata.yaml` file under the `ghpc.validators` field. These are primarily used for early validation of module-specific input variables before any infrastructure is provisioned.

### Regex Validator
The `regex` validator ensures that input variables match a specific regular expression pattern. This is commonly used to enforce Google Cloud naming conventions or specific software requirements (like Slurm partition name lengths).

**Example definition in `metadata.yaml`:**

```yaml
ghpc:
  validators:
    - validator: regex
      inputs:
        vars: [partition_name]
        pattern: "^[a-z0-9]{1,10}$"
      error_message: "partition_name must be lowercase alphanumeric and max 10 characters."
```

Unlike blueprint-level validators, these are intrinsic to the module and ensure that the module receives data in the exact format required for its internal logic to function.

## Skipping or Disabling Validators

The methods for managing or skipping validation checks vary depending on whether the validator is defined at the blueprint or module level.

### Skipping Blueprint-level Validators
For global environment checks, you can use the following methods to skip specific validators:

* Set `skip` value in validator config:

```yaml
validators:
- validator: test_apis_enabled
  inputs: {}
  skip: true
```

* Use `skip-validators` CLI flag:

```shell
./gcluster create ... --skip-validators="test_project_exists,test_apis_enabled"
```

### Disabling Module-level Validators
Module-scoped validators are defined and managed within the module's `metadata.yaml`. To control them, you must edit the module source directly.

#### To Disable a Validator
To stop a module-level validator from running, **remove** the entry from the list or **comment it out** as shown below:

```yaml
# community/modules/<category>/<module>/metadata.yaml
ghpc:
  validators: [] # Option 1: Set to an empty list
  # Option 2: Comment out the specific validator
  # - validator: regex
  #   inputs:
  #     vars: [partition_name]
  #     pattern: "^[a-z0-9]{1,10}$"
```

### Disabling All Validation (Universal)
To bypass all validation checks—including both blueprint-level and module-level validators—you can set the [validation level](#validation-levels) to `IGNORE`. This provides a single command-line toggle to suppress all automated checks.

* Set the validation level to IGNORE via CLI:

    ```shell
    ./gcluster create -l IGNORE examples/hpc-slurm.yaml
    ```

### Validation levels

Validation levels determine how the toolkit handles a validation failure. These levels apply to both Blueprint-level and Module-level validators.

They can be set to 3 differing levels of behavior using the command-line
`--validation-level` flag:

* `"ERROR"` (default): If any validator fails, the deployment directory will not be
  written. Error messages will be printed to the screen that indicate which
  validator(s) failed and how.
* `"WARNING"`: The deployment directory will be written even if any
  validators fail. Warning messages will be printed to the screen that indicate
  which validator(s) failed and how.
* `"IGNORE"`: Do not execute any validators, even if they are explicitly defined
  in a `validators` block or the default set is implicitly added.

For example, this command will set all validators to `WARNING` behavior:

```shell
./gcluster create --validation-level WARNING examples/hpc-slurm.yaml
```

The flag can be shortened to `-l` as shown below using `IGNORE` to disable all
validators.

```shell
./gcluster create -l IGNORE examples/hpc-slurm.yaml
```
