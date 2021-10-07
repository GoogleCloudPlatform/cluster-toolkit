# gHPC Commands

## Create
`ghpc create` takes as input a config file and creates a blueprint directory
based on the requested components that can be used to deploy an HPC cluster on
GCP

## Expand
`ghpc expand` takes as input a config file and expands all the fields necessary
to create a blueprint without actually creating the blueprint itself. The output
yaml can be used for debugging purposes and can be used as input to `ghpc
create`.
