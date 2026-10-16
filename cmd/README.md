# Cluster Toolkit (formerly HPC Toolkit) Commands

## gcluster

`gcluster` is the tool used by Cluster Toolkit to create deployments of AI/ML and HPC
clusters, also referred to as the gHPC Engine.

### Usage - gcluster

```bash
gcluster [FLAGS]
gcluster [SUBCOMMAND]
```

### Subcommands - gcluster

* [`deploy`](#gcluster-deploy): Deploy an AI/ML or HPC cluster on Google Cloud
* [`create`](#gcluster-create): Create a new deployment
* [`expand`](#gcluster-expand): Expand the blueprint without creating a new deployment
* [`destroy`](#gcluster-destroy): Destroys all resources in a Toolkit deployment directory
* [`export-outputs`](#gcluster-export-outputs): Export outputs from deployment group
* [`import-inputs`](#gcluster-import-inputs): Import input values from previous deployment groups
* [`cluster`](#gcluster-cluster): Discover, list, and inspect target clusters and environments (`list`, `describe`, `info`, `volume`)
* [`job`](#gcluster-job): Manage workloads on GKE clusters (`submit`, `list`, `logs`, `cancel`, `config`, `inspect`, `gke-template-extract`)
* [`telemetry`](#gcluster-telemetry): Enable or disable telemetry
* [`completion`](#gcluster-completion): Generate completion script
* [`help`](#gcluster-help): Display help information for any command

### Flags - gcluster

* `--download-dependencies`: Automatically download missing dependencies (Terraform and Packer). Pass `--download-dependencies=false` to fail if missing.
* `-h, --help`: displays detailed help for the gcluster command.
* `--no-color`: Disable colorized output.
* `-v, --version`: displays the version of gcluster being used.

### Example - gcluster

```bash
gcluster --version
```

## gcluster deploy

`gcluster deploy` deploys a cluster on Google Cloud using the deployment directory created by `gcluster create` or creates one from a supplied blueprint file.

### Usage - deploy

```bash
gcluster deploy (<DEPLOYMENT_DIRECTORY> | <BLUEPRINT_FILE>) [flags]
```

### Flags - deploy

* `--add-creator-label`: Add label `ghpc_creator` to the expanded blueprint. Defaults to `true` for `@google.com` accounts.
* `-a, --artifacts <string>`: Artifacts directory (automatically configured if unset).
* `--auto-approve`: Automatically approve proposed changes without prompting.
* `--backend-config <strings>`: Comma-separated list of `name=value` variables to set Terraform backend configuration. Can be used multiple times.
* `-d, --deployment-file <string>`: Deployment file to override blueprint variables and backend configuration.
* `--force`: Forces overwrite of existing deployment directory. If set, `--overwrite-deployment` is implied and no validation is performed on the existing deployment directory.
* `--json-output`: Write Terraform output to JSON file (instead of default English text to console).
* `--only <strings>`: Only apply to groups with the given names (comma-separated).
* `-o, --out <string>`: Sets the output directory where the deployment directory will be created.
* `-w, --overwrite-deployment`: If specified, an existing deployment directory is overwritten by the new deployment.
* `--parallelism <int>`: Limit the number of concurrent operations in Terraform (default: 10, or `GCLUSTER_TERRAFORM_PARALLELISM`).
* `--skip <strings>`: Skip groups with the given names (comma-separated).
* `--skip-validators <strings>`: Validators to skip (comma-separated).
* `-l, --validation-level <string>`: Set validation level to one of (`"ERROR"`, `"WARNING"`, `"IGNORE"`) (default `"ERROR"`).
* `--vars <strings>`: Comma-separated list of `name=value` variables to override YAML configuration. Can be used multiple times.

Refer to the [Selective Deployment and Exclusion Guide](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples/machine-learning/README.md#selective-deployment-and-destruction-using---only-and---skip-flags) for more information on managing or skipping specific group deployments.

## gcluster create

`gcluster create` creates a deployment directory. This deployment directory is used to deploy a cluster on Google Cloud.

### Usage - create

```sh
gcluster create <BLUEPRINT_FILE> [flags]
```

### Positional arguments - create

`BLUEPRINT_FILE`: the name of the blueprint file that is used for the deployment.

### Flags - create

* `--add-creator-label`: Add label `ghpc_creator` to the expanded blueprint. Defaults to `true` for `@google.com` accounts.
* `--backend-config strings`: Comma-separated list of `name=value` variables to set Terraform backend configuration. Can be used multiple times.
* `-d, --deployment-file string`: Deployment file to override blueprint variables and backend configuration.
* `--force`: Forces overwrite of existing deployment directory. If set, `--overwrite-deployment` is implied and no validation is performed on the existing deployment directory.
* `-h, --help`: display detailed help for the create command.
* `-o, --out string`: sets the output directory where the AI/ML or HPC deployment directory will be created.
* `-w, --overwrite-deployment`: If specified, an existing deployment directory is overwritten by the new deployment.

  * Terraform state IS preserved.
  * Terraform workspaces are NOT supported (behavior undefined).
  * Packer is NOT supported.

* `--skip-validators strings`: Validators to skip (comma-separated).
* `-l, --validation-level string`: sets validation level to one of (`"ERROR"`, `"WARNING"`, `"IGNORE"`) (default `"ERROR"`).
* `--vars strings`: comma-separated list of name=value variables to override YAML configuration. Can be used multiple times. Arrays or maps containing comma-separated values must be enclosed in double quotes. The double quotes may require escaping depending on the shell used. Examples below have been tested using a `bash` shell:

  * `--vars foo=bar,baz=2`
  * `--vars bar=2 --vars baz=3.14`
  * `--vars foo=true`
  * `--vars "foo={bar: baz}"`
  * `--vars "\"foo={bar: baz, qux: quux}\""`
  * `--vars "\"foo={bar: baz}\"",\"b=[foo,3,3.14]\"`
  * `--vars "\"a={foo: [bar, baz]}\"",\"b=[foo,3,3.14]\"`
  * `--vars \"b=[foo,3,3.14]\"`
  * `--vars \"b=[[foo,bar],3,3.14]\"`

### Example - create

For example to create a deployment folder using a blueprint named `my-blueprint`,
run the following command:

```bash
gcluster create my-blueprint
```

## gcluster expand

`gcluster expand` takes as input a blueprint file and expands all the fields
necessary to create a deployment without actually creating the deployment
directory. It outputs an expanded blueprint, which can be used for debugging
purposes and can be used as input to `gcluster create`.

### Usage - expand

```bash
gcluster expand BLUEPRINT_NAME [flags]
```

### Flags - expand

* `--add-creator-label`: Add label `ghpc_creator` to the expanded blueprint. Defaults to `true` for `@google.com` accounts.
* `--backend-config strings`: Comma-separated list of `name=value` variables to set Terraform backend configuration. Can be used multiple times.
* `-d, --deployment-file string`: Deployment file to override blueprint variables and backend configuration.
* `-o, --out string`: Output file for the expanded HPC Environment Definition (default `"expanded.yaml"`).
* `--skip-validators strings`: Validators to skip (comma-separated).
* `-l, --validation-level string`: Set validation level to one of (`"ERROR"`, `"WARNING"`, `"IGNORE"`) (default `"ERROR"`).
* `--vars strings`: Comma-separated list of `name=value` variables to override YAML configuration. Can be used multiple times.

## gcluster export-outputs

`gcluster export-outputs` exports Terraform output values from a deployment group for consumption by subsequent deployment groups (e.g., `gcluster export-outputs a3ultra-slurm/cluster-env`).

### Usage - export-outputs

```bash
gcluster export-outputs DEPLOYMENT_GROUP_DIRECTORY [flags]
```

## gcluster import-inputs

`gcluster import-inputs` imports output values exported by previous deployment groups as input `.tfvars` for subsequent deployment groups (e.g., `gcluster import-inputs a3ultra-slurm/cluster`).

### Usage - import-inputs

```bash
gcluster import-inputs DEPLOYMENT_GROUP_DIRECTORY [flags]
```

## gcluster cluster

`gcluster cluster` discovers, lists, and inspects target clusters and storage volumes in a Google Cloud project (`list`, `describe`, `info`, `volume`).

### Usage - cluster

```bash
gcluster cluster <list|describe|info|volume> [flags]
```

## gcluster job

`gcluster job` submits, monitors, and manages batch and distributed workloads on GKE clusters (`submit`, `list`, `logs`, `cancel`, `config`, `inspect`, `gke-template-extract`). For detailed usage, see [`docs/gcluster_job_guide.md`](../docs/gcluster_job_guide.md).

### Usage - job

```bash
gcluster job <submit|list|logs|cancel|config|inspect|gke-template-extract> [flags]
```

## gcluster telemetry

`gcluster telemetry` enables or disables anonymous usage telemetry (`on` or `off`).

### Usage - telemetry

```bash
gcluster telemetry [on|off] [flags]
```

## gcluster completion

Generates a script that enables command completion for `gcluster` for a given shell.

For detailed usage information, run `gcluster help completion`

## gcluster help

`gcluster help` prints the usage information for `gcluster` and subcommands of `gcluster`.

To generate usage details for `gcluster`, run `gcluster help`. To generate usage
details for a specific command, for example `expand`, run the following command:

```bash
gcluster help expand
```

## gcluster destroy

`gcluster destroy` destroys all resources in a Toolkit deployment directory.

### Usage - destroy

```bash
gcluster destroy DEPLOYMENT_DIRECTORY [flags]
```

### Flags - destroy

* `-a, --artifacts <string>`: Artifacts directory (automatically configured if unset).
* `--auto-approve`: Automatically approve proposed changes without prompting.
* `--only <strings>`: Only destroy groups with the given names (comma-separated).
* `--skip <strings>`: Skip destroying groups with the given names (comma-separated).
* `--robust`: Perform a robust destroy, including firewall rule cleanup.
* `--parallelism <int>`: Limit the number of concurrent operations in Terraform (default: 10, or `GCLUSTER_TERRAFORM_PARALLELISM`).

Refer to the [Selective Deployment and Exclusion Guide](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/examples/machine-learning/README.md#selective-deployment-and-destruction-using---only-and---skip-flags) for more information on managing or skipping specific group destruction.
