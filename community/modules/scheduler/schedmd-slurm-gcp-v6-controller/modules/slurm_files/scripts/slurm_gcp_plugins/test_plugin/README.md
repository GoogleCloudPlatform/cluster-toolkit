# Test slurm_gcp_plugin plugin

## Overview

This is a very basic but still useful test plugin that records the VM instance
id of the nodes used for jobs (when dynamic nodes are used).

## Callbacks used

### post_main_resume_nodes

Used to log the instance id of created VMs

### register_instance_information_fields

Used to add the instance id to the information collected for VM instances.
