# max_hops slurm_gcp_plugin plugin

## Overview

This plugin allows placement parameters to be set controlling the max number of
network hops between nodes in dynamic jobs.

## Usage

### Configuration

This plugin can be enabled by adding the following to the slurm-gcp config:

```yaml
enable_slurm_gcp_plugins:
  #possibly other plugins
  max_hops:
    max_hops: 1
```

to set the default max_hops to, in this example, 1 for _all_ jobs.

### Per job setting

The max hops setting can be changed on a per job basis using the --prefer
argument e.g. as follows:

salloc --prefer=max_hops.max_hops=1

to allow at most one network hop. For this to work the
`ignore_prefer_validation` needs to be added to the slurm `SchedulerParameters`
configuration item.

## Callbacks used

### pre_placement_group_insert

Used to change the placement group creation request.
