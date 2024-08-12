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
* [`completion`](#gcluster-completion): Generate completion script
* [`help`](#gcluster-help): Display help information for any command

### Flags - gcluster

* `-h, --help`: displays detailed help for the gcluster command.
* `-v, --version`: displays the version of gcluster being used.

### Example - gcluster

```bash
gcluster --version
```

## gcluster deploy

`gcluster deploy` deploys a cluster on Google Cloud using the deployment directory created by `gcluster create` or creates one from supplied blueprint file.

### Usage - deploy

```bash
gcluster deploy (<DEPLOYMENT_DIRECTORY> | <BLUEPRINT_FILE>) [flags]
```

## gcluster create

`gcluster create` creates a deployment directory. This deployment directory is used to deploy a cluster on Google Cloud.

### Usage - create

```sh
gcluster create BLUEPRINT_FILE [FLAGS]
```

### Positional arguments - create

`BLUEPRINT_FILE`: the name of the blueprint file that is used for the deployment.

### Flags - create

* `--backend-config strings`: Comma-separated list of name=value variables to set Terraform backend configuration. Can be used multiple times.
* `-h, --help`: display detailed help for the create command.
* `-o, --out string`: sets the output directory where the AI/ML or HPC deployment directory will be created.
* `-w, --overwrite-deployment`: If specified, an existing deployment directory is overwritten by the new deployment.

  * Terraform state IS preserved.
  * Terraform workspaces are NOT supported (behavior undefined).
  * Packer is NOT supported.

* `-l, --validation-level string`: sets validation level to one of ("ERROR", "WARNING", "IGNORE") (default "WARNING").
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

For detailed usage information, run `gcluster help create`.

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
