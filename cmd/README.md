# gHPC Commands

## Usage

```text
gHPC provides a flexible and simple to use interface to accelerate
HPC deployments on the Google Cloud Platform.

Usage:
  ghpc [flags]
  ghpc [command]

Available Commands:
  completion  Generate completion script
  create      Create a new deployment.
  expand      Expand the Environment Blueprint.
  help        Help about any command

Flags:
  -h, --help   help for ghpc

Use "ghpc [command] --help" for more information about a command.
```

## Create

`ghpc create` takes as input a blueprint file and creates a deployment directory
based on the requested components that can be used to deploy an HPC cluster on
GCP.

### Create Usage

```text
Create a new deployment based on a provided blueprint.

Usage:
  ghpc create FILENAME [flags]

Flags:
      --backend-config strings    Comma-separated list of name=value variables to set Terraform backend configuration.
                                  Can be invoked multiple times.
  -h, --help                      help for create
  -o, --out string                Output dir under which the HPC deployment dir will be created
  -w, --overwrite-deployment      if set, an existing deployment dir can be overwritten by the newly created deployment. 
                                  Note: Terraform state IS preserved. 
                                  Note: Terraform workspaces are NOT supported (behavior undefined). 
                                  Note: Packer is NOT supported.
  -l, --validation-level string   Set validation level to one of ("ERROR", "WARNING", "IGNORE") (default "WARNING")
      --vars strings              Comma-separated list of name=value variables to override YAML configuration. Can be
                                  invoked multiple times.
```

## Expand

`ghpc expand` takes as input a blueprint file and expands all the fields
necessary to create a deployment without actually creating the deployment
directory. It outputs an expanded blueprint, which can be used for debugging
purposes and can be used as input to `ghpc create`.

## Completion
Generates a script that enables command completion for `ghpc` for a given shell.

## Help
`ghpc help` prints the usage information for `ghpc` and subcommands of `ghpc`.

To generate usage details for `ghpc`, simply run `ghpc help`. To generate usage
details for a specific command, for example `expand`, run the following command:

```bash
ghpc help expand
```
