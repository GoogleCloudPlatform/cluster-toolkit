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
* `test_machine_type_in_zone`
  * Inputs: `project_id` (string), `zone` (string), `machine_type` (string)
  * PASS: If the machine type is available in the specified zone and project.
  * SKIP (Soft Warning): If the Compute Engine API is disabled or the credentials lack `compute.machineTypes.get` permissions, the validator prints a warning and the check is skipped.
  * FAIL: If the machine type is invalid or unavailable in that zone.
  * Note: To explicitly verify multiple machine types in a zone, add this validator to the blueprint multiple times.
  * Manual test: `gcloud compute machine-types describe $(vars.machine_type) --zone $(vars.zone) --project $(vars.project_id)`
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
  - validator: test_machine_type_in_zone
    inputs:
      project_id: $(vars.project_id)
      zone: $(vars.zone)
      machine_type: c2-standard-60  # any machine type to verify in the zone
```

## Module-level (Metadata) Validators

Module-level validators are defined directly within a module's `metadata.yaml` file under the `ghpc.validators` field. These are primarily used for early validation of module-specific input variables before any infrastructure is provisioned.

By default, a failure in a module-level validator will stop execution and return an error. You can make a validator optional by setting the `level` field to `warning`. In this case, a failure will print a warning message but allow the toolkit to continue.

**Example definition in `metadata.yaml`:**

```yaml
ghpc:
  validators:
  - validator: range
    inputs:
      vars: [versions]
      min: 1
    error_message: "The 'versions' list must contain at least one version."
    level: warning # Optional: failure issues a warning instead of an error
```

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

### Allowed Enum Validator
The `allowed_enum` validator ensures that user-provided settings conform to a predefined list of allowed values (enums). Supports optional `case_sensitive` (defaults to true) and `allow_null` (defaults to false) flags.

**Example definition in `metadata.yaml`:**

```yaml
ghpc:
  validators:
  - validator: allowed_enum
    inputs:
      vars: [network_routing_mode]
      allowed: [GLOBAL, REGIONAL]
      case_sensitive: false
      allow_null: false
    error_message: "'network_routing_mode' must be GLOBAL or REGIONAL."
```

### Range Validator
The `range` validator ensures input variables either their values or lengths fall within specified numerical minimum and/or maximum bounds. It supports validating individual numeric values, lists of numeric values, and the number of elements in a list. The optional length_check field (defaulting to false) determines whether to validate the values themselves or the length of the variable.

**Example definition in `metadata.yaml`:**

```yaml
ghpc:
  validators:
  - validator: range
    inputs:
      vars: [versions]
      min: 1
      max: 8
      length_check: true # enables validation of the list's length rather than the individual values it contains.
    error_message: "The 'versions' list must contain at least one version."
```

### Exclusive Validator
The `exclusive` validator ensures that at most one of the specified variables is set. It treats variables as 'set' if they are non-empty strings, non-zero numbers, true booleans, or non-empty lists/maps.

**Example definition in `metadata.yaml`:**

```yaml
ghpc:
validators:
  - validator: exclusive
    inputs:
      vars: [preemptible, reserved]
    error_message: "'preemptible' and 'reserved' are mutually exclusive and both cannot be set at the same time."
```

### Required Validator
The `required` validator ensures that a specific set of variables are either present or absent depending on the deprecated flag. It is used to enforce mandatory inputs or to block restricted and deprecated configurations.

vars (list of strings): The list of variable names to check.
deprecated (boolean, optional): If true, the validator checks that the specified variables are not set. Defaults to false (checks that variables are set).

**Example: Enforcing required variables definition in `metadata.yaml`:**

```yaml
ghpc:
  validators:
  - validator: required
    inputs:
      vars: [vpc_network_name, subnetwork_name]
    error_message: "Network details must be provided."
```

**Example: Deprecated variables definition in `metadata.yaml`:**

```yaml
ghpc:
  validators:
  - validator: required
    inputs:
      vars: [legacy_option]
      deprecated: true
    error_message: "The 'legacy_option' is no longer supported."
```

### Conditional Validator
The `conditional` validator enforces that a dependent variable is set or matches a specific value only when a trigger variable condition is met. This is useful for cross-variable dependencies (e.g., if feature X is enabled, setting Y is required).

If trigger_value or dependent_value is omitted, the validator checks if the variable is simply "set" (non-null, true bool, positive integer, and non-empty list/tuple/map).
If a value is provided, it must match exactly. This also supports matching against null to check if a variable is explicitly omitted.

**Example definition in `metadata.yaml`:**

```yaml
ghpc:
  validators:
    - validator: conditional
      inputs:
        trigger: enable_hybrid
        trigger_value: true
        dependent: slurm_control_host
      error_message: "slurm_control_host is required when enable_hybrid is true."
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

>**Note:** The individual `level: warning` setting allows specific rules to be optional even when the global `--validation-level` is set to `ERROR`. However, the global `--validation-level IGNORE` flag will skip all validators regardless of their individual settings.

For example, this command will set all validators to `WARNING` behavior:

```shell
./gcluster create --validation-level WARNING examples/hpc-slurm.yaml
```

The flag can be shortened to `-l` as shown below using `IGNORE` to disable all
validators.

```shell
./gcluster create -l IGNORE examples/hpc-slurm.yaml
```
