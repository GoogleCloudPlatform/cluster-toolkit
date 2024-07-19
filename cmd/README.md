# HPC Toolkit Commands

## ghpc

`ghpc` is the tool used by Cloud HPC Toolkit to create deployments of HPC
clusters, also referred to as the gHPC Engine.

### Usage - ghpc

```bash
ghpc [FLAGS]
ghpc [SUBCOMMAND]
```

### Subcommands - ghpc

* [`deploy`](#ghpc-deploy): Deploy an HPC cluster on Google Cloud
* [`create`](#ghpc-create): Create a new deployment
* [`expand`](#ghpc-expand): Expand the blueprint without creating a new deployment
* [`completion`](#ghpc-completion): Generate completion script
* [`help`](#ghpc-help): Display help information for any command

### Flags - ghpc

* `-h, --help`: displays detailed help for the ghpc command.
* `-v, --version`: displays the version of ghpc being used.

### Example - ghpc

```bash
ghpc --version
```

## ghpc deploy

`ghpc deploy` deploys an HPC cluster on Google Cloud using the deployment directory created by `ghpc create` or creates one from supplied blueprint file.

### Usage - deploy

```bash
ghpc deploy (<DEPLOYMENT_DIRECTORY> | <BLUEPRINT_FILE>) [flags]
```

## ghpc create

`ghpc create` creates a deployment directory. This deployment directory is used to deploy an HPC cluster on Google Cloud.

### Usage - create

```sh
ghpc create BLUEPRINT_FILE [FLAGS]
```

### Positional arguments - create

`BLUEPRINT_FILE`: the name of the blueprint file that is used for the deployment.

### Flags - create

* `--backend-config strings`: Comma-separated list of name=value variables to set Terraform backend configuration. Can be used multiple times.
* `-h, --help`: display detailed help for the create command.
* `-o, --out string`: sets the output directory where the HPC deployment directory will be created.
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
ghpc create my-blueprint
```

## ghpc expand

`ghpc expand` takes as input a blueprint file and expands all the fields
necessary to create a deployment without actually creating the deployment
directory. It outputs an expanded blueprint, which can be used for debugging
purposes and can be used as input to `ghpc create`.

For detailed usage information, run `ghpc help create`.

## ghpc completion
Generates a script that enables command completion for `ghpc` for a given shell.

For detailed usage information, run `ghpc help completion`

## ghpc help
`ghpc help` prints the usage information for `ghpc` and subcommands of `ghpc`.

To generate usage details for `ghpc`, run `ghpc help`. To generate usage
details for a specific command, for example `expand`, run the following command:

```bash
ghpc help expand
```
