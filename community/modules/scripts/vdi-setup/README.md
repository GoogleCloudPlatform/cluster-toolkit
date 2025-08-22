## Description

Creates a containerised Guacamole instance. Currently designed to work mainly with the Rocky image in the blueprint example below. Ubuntu/Debian support previously worked too but fixes may be required due to updates.

## Features

- **VDI Tool Support**: Currently supports Guacamole for web-based VDI access
- **User Provisioning**: Supports local user creation with secure password management and group management
- **VNC Integration**: Configures VNC servers for desktop access with port change support
- **Secret Manager Integration**: Secure password storage and retrieval for VDI users and webapp admin
- **VDI Monitoring System**: Automatic monitoring and reconfiguration capabilities with password reset flags and deployment status updates. [More info](roles/vdi_monitor/README.md).
- **Debug Mode**: Comprehensive logging when enabled via the `debug` variable
- **Port Change Support**: Handles both webapp port changes (container recreation) and user VNC port changes (SQL updates)
- **Resolution Control**: Configurable VDI resolution with browser scaling control via `vdi_resolution_locked` parameter

## Secret Manager Integration

The module integrates with Google Cloud Secret Manager for secure password handling:

- **VDI Users**: Passwords are stored in Secret Manager with the pattern `vdi-user-password-{username}-{deployment_name}`
- **Webapp Admin**: Password is stored in Secret Manager with the pattern `webapp-server-password-{deployment_name}` (always stored, not just when reset is enabled)
- **Password Sources**: Users can specify `secret_name` to fetch existing passwords, provide `password` directly, or let the system generate random passwords
- **Reset Functionality**: Both individual user passwords (`reset_password`) and webapp admin password (`reset_webapp_admin_password`) can be forced to regenerate
- **Database Updates**: Password resets update existing database records without re-initializing the entire database

## Port Change and User Group Management

The module supports dynamic configuration changes without full redeployment:

### **Webapp Port Changes**
- **Container Recreation**: When `vdi_webapp_port` changes, the Guacamole webapp container is recreated with the new port
- **Database Synchronization**: Database passwords are automatically synchronized during port changes
- **Service Continuity**: VNC services remain unaffected during webapp port changes

### **User VNC Port Changes**
- **SQL Updates**: User VNC port changes are handled via database updates without container recreation
- **VNC Service Management**: VNC services are automatically stopped and restarted with new ports
- **X Server Cleanup**: Old X server processes are properly terminated before starting new ones

### **User Group Management**
- **Dynamic Group Changes**: When `vdi_user_group` changes, users are automatically migrated to the new group
- **Group Removal**: Users are removed from the old group and added to the new group
- **Idempotent Operations**: Group changes are safe to apply multiple times

## VDI Monitoring System

The module includes a monitoring system that:

- **Deployment Status**: Updates instance metadata to reflect deployment state (`available`, `reconfiguring`, `failed`)
- **Targeted Updates**: Performs database-only updates for password resets and user changes without container recreation
- **Password Reset Flags**: Supports `reset_password` for individual users and `reset_webapp_admin_password` for the webapp admin account
- **Automatic Reconfiguration**: Detects changes and triggers reconfiguration when needed
- **Status Tracking**: Maintains deployment status through instance metadata

## Usage

### Basic Configuration

```yaml
blueprint_name: vdi-test

vars:
  deployment_name: vdi-test
  project_id: * project name here *
  region: us-central1
  zone: us-central1-a

deployment_groups:
- group: primary
  modules:
  - id: network1
    source: modules/network/vpc
    settings:
      extra_iap_ports: [8080]
      firewall_rules:
      - name: allow-guacamole-8080-ext
        description: Allow external ingress to Guacamole on TCP port 8080
        direction: INGRESS
        ranges: ["0.0.0.0/0"]
        allow:
          - protocol: tcp
            ports: ["8080"]

  - id: enable-apis
    source: community/modules/project/service-enablement
    settings:
      gcp_service_list:
      - secretmanager.googleapis.com
      - storage.googleapis.com
      - compute.googleapis.com

  - id: vdi-setup
    source: community/modules/scripts/vdi-setup
    settings:
      vnc_flavor: tigervnc
      vdi_tool: guacamole
      vdi_user_group: vdiusers
      vdi_resolution: 1920x1080
      vdi_resolution_locked: true
      # Enable debug mode for verbose logging
      debug: true
      # Force reset of webapp admin password (optional)
      reset_webapp_admin_password: false
      user_provision: local_users
      vdi_users:
      # Alice: password generated and saved to Secret Manager in deployment project
      - username: alice
        port: 5901
      # Bob: existing password is retrieved from Secret Manager in deployment project
      - username: bob
        port: 5902
        secret_name: a-password-for-bob
      # Charlie: password retrieved from Secret Manager in a different project
      - username: charlie
        port: 5903
        secret_name: charlie-password
        secret_project: another-project-id
      # David: auto-generated password with reset flag (triggers password regeneration)
      - username: david
        port: 5904
        reset_password: true

  - id: guac_vm
    source: modules/compute/vm-instance
    settings:
      instance_image:
        family: hpc-rocky-linux-8
        project: cloud-hpc-image-public
        #family: debian-11
        #project: debian-cloud
        #family: ubuntu-2204-lts
        #project: ubuntu-os-cloud
        name_prefix: guacamole
        add_deployment_name_before_prefix: true
      machine_type: e2-highcpu-8
      tags: ["guacamole"]
    use:
     - network1
     - vdi-setup
```

[NVIDIA vWS drivers](https://cloud.google.com/compute/docs/gpus/grid-drivers-table) will be automatically installed on [supported machine types](https://cloud.google.com/compute/docs/gpus#gpu-virtual-workstations).

Important note: Before deploying the above example you would need to ensure the `secret_name` exists in your local project, and that your service account has sufficient access permissions if retrieving the secret from a separate `secret_project`. The following two example commands show how you can create the two example user's respective secrets:

```bash
# Create secrets for each user in the deployment project
echo -n "BobPassword123" | gcloud secrets create a-password-for-bob --data-file=-

# Or create secrets in a different project (if using secret_project)
echo -n "CharliePassword123" | gcloud secrets create charlie-password --data-file=- --project=another-project-id
```

### Accessing VDI

After deployment, you can access the VDI in several ways:

1. **Web Interface** (for Guacamole):
   - Access web interface:
     - http://$VM_PUBLIC_IP:8080/guacamole/#/
     - Note: It is not advisable to serve the web interface directly from a public IP in production environments. You should consider placing the VDI behind a reverse proxy, load balancer, or tunnel to it directly over IAP (see below).

   - Admin credentials:
     - Username: `guacadmin` (for Guacamole)
     - Password: Retrieve the `webapp-server-password-{deployment_name}` secret from Secret Manager

2. **User VDI Access**:
   - Each user's credentials are stored in Secret Manager
   - Secret names follow the pattern: `vdi-user-password-{username}-{deployment_name}` for auto-generated passwords
   - Or use the custom secret name specified in the `vdi_users` configuration

3. **IAP Tunnel** (for development/testing):

   ```bash
   gcloud compute start-iap-tunnel vdi-test-0 8080 \
       --local-host-port=localhost:8080 \
       --zone=us-central1-a
   ```

   - The web interface will then be accessible from http://localhost:8080/guacamole/ (for Guacamole)

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0 |
| <a name="requirement_archive"></a> [archive](#requirement\_archive) | ~> 2.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_archive"></a> [archive](#provider\_archive) | ~> 2.0 |
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_startup_script"></a> [startup\_script](#module\_startup\_script) | ../../../../modules/scripts/startup-script | n/a |

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket.bucket](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [terraform_data.input_validation](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [archive_file.roles_tar](https://registry.terraform.io/providers/hashicorp/archive/latest/docs/data-sources/file) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_debug"></a> [debug](#input\_debug) | Enable debug mode for verbose logging during VDI setup. | `bool` | `false` | no |
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the deployment. | `string` | n/a | yes |
| <a name="input_force_rerun"></a> [force\_rerun](#input\_force\_rerun) | Force complete container recreation and database re-initialization, bypassing all idempotency checks. Use only when troubleshooting or when the system is in a broken state. | `bool` | `false` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Key-value pairs of labels to be added to created resources. | `map(string)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region to place bucket containing startup script. | `string` | n/a | yes |
| <a name="input_reset_webapp_admin_password"></a> [reset\_webapp\_admin\_password](#input\_reset\_webapp\_admin\_password) | Force reset of the webapp admin password during reconfiguration. If true, a new password will be generated and stored in Secret Manager, even if an existing password exists. | `bool` | `false` | no |
| <a name="input_user_provision"></a> [user\_provision](#input\_user\_provision) | User type to create (local\_users supported. os-login to do. | `string` | `"local_users"` | no |
| <a name="input_vdi_resolution"></a> [vdi\_resolution](#input\_vdi\_resolution) | Desktop resolution for VNC sessions (e.g. 1920x1080). | `string` | `"1920x1080"` | no |
| <a name="input_vdi_resolution_locked"></a> [vdi\_resolution\_locked](#input\_vdi\_resolution\_locked) | Disable resize of remote display in Guacamole connections. When true, VDI displays at native resolution without browser scaling. | `bool` | `true` | no |
| <a name="input_vdi_tool"></a> [vdi\_tool](#input\_vdi\_tool) | VDI tool to deploy (guacamole currently supported). | `string` | `"guacamole"` | no |
| <a name="input_vdi_user_group"></a> [vdi\_user\_group](#input\_vdi\_user\_group) | Unix group to create/use for VDI users. | `string` | `"vdiusers"` | no |
| <a name="input_vdi_users"></a> [vdi\_users](#input\_vdi\_users) | List of VDI users to configure. Passwords are handled securely by the Ansible roles: if secret\_name is provided, the password is fetched from Secret Manager; if neither password nor secret\_name is provided, a random password is generated and stored in Secret Manager. If secret\_project is provided, it specifies the GCP project where the secret is stored (defaults to the deployment project). Set reset\_password to true to trigger password regeneration for auto-generated passwords. | <pre>list(object({<br/>    username       = string<br/>    port           = number<br/>    secret_name    = optional(string)<br/>    secret_project = optional(string)<br/>    reset_password = optional(bool)<br/>  }))</pre> | `[]` | no |
| <a name="input_vdi_webapp_port"></a> [vdi\_webapp\_port](#input\_vdi\_webapp\_port) | Port to serve the Webapp interface from if applicable (note: containers will be recreated if changed) | `string` | `"8080"` | no |
| <a name="input_vnc_flavor"></a> [vnc\_flavor](#input\_vnc\_flavor) | The VNC server flavor to use (tigervnc currently supported) | `string` | `"tigervnc"` | no |
| <a name="input_vnc_port_max"></a> [vnc\_port\_max](#input\_vnc\_port\_max) | Maximum valid VNC port. | `number` | `5999` | no |
| <a name="input_vnc_port_min"></a> [vnc\_port\_min](#input\_vnc\_port\_min) | Minimum valid VNC port. | `number` | `5901` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Zone in which the VDI instances are created. | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_guacamole_admin_password_secret"></a> [guacamole\_admin\_password\_secret](#output\_guacamole\_admin\_password\_secret) | The name of the Secret Manager secret containing the Guacamole admin password |
| <a name="output_guacamole_admin_username"></a> [guacamole\_admin\_username](#output\_guacamole\_admin\_username) | The admin username for Guacamole |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | Combined startup script that installs VDI (VNC, Guacamole, users). |
| <a name="output_vdi_runner"></a> [vdi\_runner](#output\_vdi\_runner) | Shell runner wrapping Ansible playbook + roles (for custom-image or direct use). |
| <a name="output_vdi_user_credentials"></a> [vdi\_user\_credentials](#output\_vdi\_user\_credentials) | Map of VDI user credentials stored in Secret Manager |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
