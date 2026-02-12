<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.41 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.41 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_instance"></a> [instance](#module\_instance) | ../instance | n/a |
| <a name="module_template"></a> [template](#module\_template) | ../instance_template | n/a |

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket_object.config](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |
| [google_storage_bucket_object.startup_scripts](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket_object) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_internal_startup_script"></a> [internal\_startup\_script](#input\_internal\_startup\_script) | FOR INTERNAL TOOLKIT USAGE ONLY. | `string` | `null` | no |
| <a name="input_login_nodes"></a> [login\_nodes](#input\_login\_nodes) | Slurm login instance definitions. | <pre>object({<br/>    group_name = string<br/>    access_config = optional(list(object({<br/>      nat_ip       = string<br/>      network_tier = string<br/>    })))<br/>    additional_disks = optional(list(object({<br/>      disk_name                  = optional(string)<br/>      device_name                = optional(string)<br/>      disk_size_gb               = optional(number)<br/>      disk_type                  = optional(string)<br/>      disk_labels                = optional(map(string), {})<br/>      auto_delete                = optional(bool, true)<br/>      boot                       = optional(bool, false)<br/>      disk_resource_manager_tags = optional(map(string), {})<br/>    })), [])<br/>    additional_networks = optional(list(object({<br/>      access_config = optional(list(object({<br/>        nat_ip       = string<br/>        network_tier = string<br/>      })), [])<br/>      alias_ip_range = optional(list(object({<br/>        ip_cidr_range         = string<br/>        subnetwork_range_name = string<br/>      })), [])<br/>      ipv6_access_config = optional(list(object({<br/>        network_tier = string<br/>      })), [])<br/>      network            = optional(string)<br/>      network_ip         = optional(string, "")<br/>      nic_type           = optional(string)<br/>      queue_count        = optional(number)<br/>      stack_type         = optional(string)<br/>      subnetwork         = optional(string)<br/>      subnetwork_project = optional(string)<br/>    })), [])<br/>    bandwidth_tier             = optional(string, "platform_default")<br/>    can_ip_forward             = optional(bool, false)<br/>    disk_auto_delete           = optional(bool, true)<br/>    disk_labels                = optional(map(string), {})<br/>    disk_resource_manager_tags = optional(map(string), {})<br/>    disk_size_gb               = optional(number)<br/>    disk_type                  = optional(string, "n1-standard-1")<br/>    enable_confidential_vm     = optional(bool, false)<br/>    enable_oslogin             = optional(bool, true)<br/>    enable_shielded_vm         = optional(bool, false)<br/>    gpu = optional(object({<br/>      count = number<br/>      type  = string<br/>    }))<br/>    labels       = optional(map(string), {})<br/>    machine_type = optional(string)<br/>    advanced_machine_features = object({<br/>      enable_nested_virtualization = optional(bool)<br/>      threads_per_core             = optional(number)<br/>      turbo_mode                   = optional(string)<br/>      visible_core_count           = optional(number)<br/>      performance_monitoring_unit  = optional(string)<br/>      enable_uefi_networking       = optional(bool)<br/>    })<br/>    metadata              = optional(map(string), {})<br/>    min_cpu_platform      = optional(string)<br/>    num_instances         = optional(number, 1)<br/>    on_host_maintenance   = optional(string)<br/>    preemptible           = optional(bool, false)<br/>    region                = optional(string)<br/>    resource_manager_tags = optional(map(string), {})<br/>    service_account = optional(object({<br/>      email  = optional(string)<br/>      scopes = optional(list(string), ["https://www.googleapis.com/auth/cloud-platform"])<br/>    }))<br/>    shielded_instance_config = optional(object({<br/>      enable_integrity_monitoring = optional(bool, true)<br/>      enable_secure_boot          = optional(bool, true)<br/>      enable_vtpm                 = optional(bool, true)<br/>    }))<br/>    source_image_family  = optional(string)<br/>    source_image_project = optional(string)<br/>    source_image         = optional(string)<br/>    static_ips           = optional(list(string), [])<br/>    subnetwork           = string<br/>    spot                 = optional(bool, false)<br/>    tags                 = optional(list(string), [])<br/>    zone                 = optional(string)<br/>    termination_action   = optional(string)<br/>  })</pre> | n/a | yes |
| <a name="input_network_storage"></a> [network\_storage](#input\_network\_storage) | Storage to mounted on login instances<br/>- server\_ip     : Address of the storage server.<br/>- remote\_mount  : The location in the remote instance filesystem to mount from.<br/>- local\_mount   : The location on the instance filesystem to mount to.<br/>- fs\_type       : Filesystem type (e.g. "nfs").<br/>- mount\_options : Options to mount with. | <pre>list(object({<br/>    server_ip     = string<br/>    remote_mount  = string<br/>    local_mount   = string<br/>    fs_type       = string<br/>    mount_options = string<br/>  }))</pre> | `[]` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project ID to create resources in. | `string` | n/a | yes |
| <a name="input_replace_trigger"></a> [replace\_trigger](#input\_replace\_trigger) | Trigger value to replace the instances. | `string` | `""` | no |
| <a name="input_slurm_bucket_dir"></a> [slurm\_bucket\_dir](#input\_slurm\_bucket\_dir) | Path to directory in the bucket for configs | `string` | n/a | yes |
| <a name="input_slurm_bucket_name"></a> [slurm\_bucket\_name](#input\_slurm\_bucket\_name) | Name of the bucket for configs | `string` | n/a | yes |
| <a name="input_slurm_bucket_path"></a> [slurm\_bucket\_path](#input\_slurm\_bucket\_path) | GCS Bucket URI of Slurm cluster file storage. | `string` | n/a | yes |
| <a name="input_slurm_cluster_name"></a> [slurm\_cluster\_name](#input\_slurm\_cluster\_name) | Cluster name | `string` | n/a | yes |
| <a name="input_startup_scripts"></a> [startup\_scripts](#input\_startup\_scripts) | List of scripts to be ran on login VMs startup. | <pre>list(object({<br/>    filename = string<br/>    content  = string<br/>  }))</pre> | `[]` | no |
| <a name="input_startup_scripts_timeout"></a> [startup\_scripts\_timeout](#input\_startup\_scripts\_timeout) | The timeout (seconds) applied to each startup script. If any script exceeds this timeout, <br/>then the instance setup process is considered failed and handled accordingly.<br/><br/>NOTE: When set to 0, the timeout is considered infinite and thus disabled. | `number` | `300` | no |
| <a name="input_universe_domain"></a> [universe\_domain](#input\_universe\_domain) | Domain address for alternate API universe | `string` | `"googleapis.com"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_instances"></a> [instances](#output\_instances) | VM instances of login nodes |
| <a name="output_service_account"></a> [service\_account](#output\_service\_account) | Service Account used by login VMs |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
