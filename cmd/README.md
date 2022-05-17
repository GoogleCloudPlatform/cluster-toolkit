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

## Expand

`ghpc expand` takes as input a blueprint file and expands all the fields
necessary to create a deployment without actually creating the deployment
directory. It outputs an expanded blueprint, which can be used for debugging
purposes and can be used as input to `ghpc create`.

## Completion
Generates a script that enables command completion for `ghpc` for a given shell.

## Help
`ghpc help` prints the usage information from above to the console.
