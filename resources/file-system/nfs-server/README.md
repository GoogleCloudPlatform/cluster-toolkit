## Description
This resource defines a file-system that builds on top of a VM, setting up the VM using 
start up script will allow the VM to act as a file system.

### Example
```
- source: ./resources/file-system/nfs-server
  kind: terraform
  id: homefs
  settings:
    network_name: <network_name>
```
This creates a file system on GCE in terraform, which outputs the required network storage information
to be accessed by the slurm cluster and mount.

## License
<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.
