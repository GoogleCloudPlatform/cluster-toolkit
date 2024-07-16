# Plugin mechanism for slurm-gcp

## Introduction

Slurm in general provides many hooks for customization of its various functions.
In fact - slurm-gcp is using one of these customization points, PrologSlurmctld,
to perform tasks related to VM instance creation as a response to job node
allocation.

The plugin mechanism in this directory similarly allows deployment specific
customizations to slurm-gcp by dropping Python modules in
`<slurm_gcp_base>/scripts/slurm_gcp_plugins` and enabling plugins setting the
configuration directive `enable_slurm_gcp_plugins = true` in
`<slurm_gcp_base>/scripts/config.yaml`

A very basic `test_plugin`, is provided as an example.

## Plugins

Callbacks to registered plugins can be made from various places in resume.py and
suspend.py. The following callbacks are currently made:

### Callback function signature

Callback functions in the plugins are recommended to be declared as follows:

```python
def post_main_resume_nodes(*pos_args, **keyword_args):
...
```

and extract arguments from `keyword_args`. Check the callback sites to
understand which values that are available.

### Current callback sites

Callbacks are currently performed from the following places:

#### scripts/resume.py:main_resume_nodes

At the end of main the following callback is called

```python
def post_main_resume_nodes(*pos_args, **keyword_args):
```

The primary intention is allow a plugin to record details about the instance
and/or setup/change properties for which the VMs needs to be up and running.

Currently the call is made regardless of if the the resume node operation
succeeded or not.

#### scripts/resume.py:create_instances_request

In create_instances_request just before the bulk instance insert is called, the
following callback is called

```python
def pre_instance_bulk_insert(*pos_args, **keyword_args):
```

The primary intention is allow a plugin to modify the instance creation request.

#### scripts/resume.py:create_placement_request

In create_instances_request just before the resource policy creation, the
following callback is called

```python
def pre_placement_group_insert(*pos_args, **keyword_args):
```

The primary intention is allow a plugin to modify the resource policy creation
request.

#### scripts/suspend.py:main_suspend_nodes

In main just before the VMs are deleted but while they still (should) exist, the
following callback is called

```python
def pre_main_suspend_nodes(*pos_args, **keyword_args):
```

The primary intention is allow a plugin to cleanup or record details while the
node still exists.

#### scripts/util.py:instances

Just before the per-instance information is requested the following callback is
called:

```python
def register_instance_information_fields(*pos_args, **keyword_args):
```

The primary intention is allow a plugin to add information to the per instance
lookup.

### Logging and error handling

Plugin functions are recommended to use `logging` to communicate information,
warnings and errors. The `slurm_gcp_plugins` registry tries to isolate the
caller of the callbacks (i.e. resume.py and suspend.py) from effects of errors
with a general try-catch wrapper for each plugin callback. However - as the
callback happens in the same process there are notable limits on how much
isolation that can be achieved.
