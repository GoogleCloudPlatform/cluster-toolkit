## Blueprint Validation

The Toolkit contains "validator" functions that perform basic tests of
the blueprint to ensure that deployment variables are valid and that the HPC
environment can be provisioned in your Google Cloud project. To succeed,
validators need the following services to be enabled in your blueprint
project(s):

* Compute Engine API (compute.googleapis.com)
* Service Usage API (serviceusage.googleapis.com)

One can [explicitly define validators](#explicit-validators), however, the
expectation is that the implicit behavior will be useful for most users. When
implicit, a validator is added if all deployment variables matching its inputs
is defined. The `test_apis_enabled` validator is always enabled because it reads
the entire blueprint and does not require any specific deployment variable. If
`project_id`, `region`, and `zone` are defined as deployment variables, then the
following validators are enabled:

```yaml
validators:
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
    zone: $(vars.zone)
    region: $(vars.region)
```

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
* `test_region_exists`
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
  * Manual test: `gcloud compute regions describe us-central1 --format="text(zones)" --project $(vars.project_id)

### Explicit validators

Validators can be overwritten and supplied with alternative input values,
however they are limited to the set of functions defined above. One method by
which to disable validators is to explicitly set them to the empty list:

```yaml
validators: []
```

### Validation levels

They can also be set to 3 differing levels of behavior using the command-line
`--validation-level` flag` for the `create` and `expand` commands:

* `"ERROR"`: If any validator fails, the deployment directory will not be
  written. Error messages will be printed to the screen that indicate which
  validator(s) failed and how.
* `"WARNING"` (default): The deployment directory will be written even if any
  validators fail. Warning messages will be printed to the screen that indicate
  which validator(s) failed and how.
* `"IGNORE"`: Do not execute any validators, even if they are explicitly defined
  in a `validators` block or the default set is implicitly added.

For example, this command will set all validators to `WARNING` behavior:

```shell
./ghpc create --validation-level WARNING examples/hpc-cluster-small.yaml
```

The flag can be shortened to `-l` as shown below using `IGNORE` to disable all
validators.

```shell
./ghpc create -l IGNORE examples/hpc-cluster-small.yaml
```
