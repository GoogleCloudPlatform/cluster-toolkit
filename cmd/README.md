# gHPC Commands

## Create

`ghpc create` takes as input a blueprint file and creates a deployment directory
based on the requested components that can be used to deploy an HPC cluster on
GCP

## Expand

`ghpc expand` takes as input a blueprint file and expands all the fields
necessary to create a deployment without actually creating the deployment
directory. It outputs an expanded blueprint, which can be used for debugging
purposes and can be used as input to `ghpc create`.
