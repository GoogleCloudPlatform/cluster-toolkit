## Description

Creates a containerised Guacamole instance. Works with the Rocky, Debian and Ubuntu images shown in blueprint example below.

### Secret Manager Integration

The VDI module supports flexible Secret Manager integration:

- **Default Behavior**: Secrets are stored in the deployment project
- **Automatic Password Generation**: If no `secret_name` is provided, random passwords are generated and stored
- **Existing Secret Retrieval**: Provide `secret_name` to use existing secrets from Secret Manager
- **Cross-Project Secrets**: Use `secret_project` to specify a different GCP project if providing secrets

## Basic Example (Guacamole)

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
      user_provision: local_users
      vdi_user_group: vdiusers
      vdi_resolution: 1920x1080
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

  - id: guac_vm
    source: modules/compute/vm-instance
    settings:
      instance_image:
        # Several supported image families:
        family: hpc-rocky-linux-8
        project: cloud-hpc-image-public
        #family: debian-11
        #project: debian-cloud
        #family: ubuntu-2204-lts
        #project: ubuntu-os-cloud
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

### Accessing Guacamole VDI

After deployment, you can access the VDI in several ways:

1. **Guacamole Web Interface**:
   - Access web interface:
     - http://$VM_PUBLIC_IP:8080/guacamole/#/
     - Note: It is not advisable to serve Guacamole directly from a public IP in production environments. You should consider placing the VDI behind a reverse proxy, load balancer, or tunnel to it directly over IAP (see below).

   - Admin credentials:
     - Username: `guacadmin`
     - Password: Retrieve the `webapp-server...` secret from Secret Manager

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

   - Guacamole will then be accessible from http://localhost:8080/guacamole/

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0 |
| <a name="requirement_archive"></a> [archive](#requirement\_archive) | ~> 2.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 3.83 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_archive"></a> [archive](#provider\_archive) | ~> 2.0 |
| <a name="provider_google"></a> [google](#provider\_google) | >= 3.83 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_startup_script"></a> [startup\_script](#module\_startup\_script) | ../../../../modules/scripts/startup-script | n/a |

## Resources

| Name | Type |
|------|------|
| [google_storage_bucket.bucket](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/storage_bucket) | resource |
| [terraform_data.input_validation](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [archive_file.roles_tar](https://registry.terraform.io/providers/hashicorp/archive/latest/docs/data-sources/file) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the deployment. | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Key-value pairs of labels to be added to created resources. | `map(string)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region to place bucket containing startup script. | `string` | n/a | yes |
| <a name="input_user_provision"></a> [user\_provision](#input\_user\_provision) | User type to create (local\_users supported. os-login to do. | `string` | `"local_users"` | no |
| <a name="input_vdi_resolution"></a> [vdi\_resolution](#input\_vdi\_resolution) | Desktop resolution for VNC sessions (e.g. 1920x1080). | `string` | `"1920x1080"` | no |
| <a name="input_vdi_tool"></a> [vdi\_tool](#input\_vdi\_tool) | VDI tool to deploy (guacamole currently supported). | `string` | `"guacamole"` | no |
| <a name="input_vdi_user_group"></a> [vdi\_user\_group](#input\_vdi\_user\_group) | Unix group to create/use for VDI users. | `string` | `"vdiusers"` | no |
| <a name="input_vdi_users"></a> [vdi\_users](#input\_vdi\_users) | List of VDI users to configure. Passwords are handled securely by the Ansible roles: if secret\_name is provided, the password is fetched from Secret Manager; if neither password nor secret\_name is provided, a random password is generated and stored in Secret Manager. If secret\_project is provided, it specifies the GCP project where the secret is stored (defaults to the deployment project). | <pre>list(object({<br/>    username       = string<br/>    port           = number<br/>    secret_name    = optional(string)<br/>    secret_project = optional(string)<br/>  }))</pre> | `[]` | no |
| <a name="input_vdi_webapp_port"></a> [vdi\_webapp\_port](#input\_vdi\_webapp\_port) | Port to serve the Webapp interface from if applicable | `string` | `"8080"` | no |
| <a name="input_vnc_flavor"></a> [vnc\_flavor](#input\_vnc\_flavor) | The VNC server flavor to use (tigervnc currently supported) | `string` | `"tigervnc"` | no |
| <a name="input_vnc_port_max"></a> [vnc\_port\_max](#input\_vnc\_port\_max) | Maximum valid VNC port. | `number` | `5999` | no |
| <a name="input_vnc_port_min"></a> [vnc\_port\_min](#input\_vnc\_port\_min) | Minimum valid VNC port. | `number` | `5901` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_guacamole_admin_password_secret"></a> [guacamole\_admin\_password\_secret](#output\_guacamole\_admin\_password\_secret) | The name of the Secret Manager secret containing the Guacamole admin password |
| <a name="output_guacamole_admin_username"></a> [guacamole\_admin\_username](#output\_guacamole\_admin\_username) | The admin username for Guacamole |
| <a name="output_startup_script"></a> [startup\_script](#output\_startup\_script) | Combined startup script that installs VDI (VNC, Guacamole, users). |
| <a name="output_vdi_runner"></a> [vdi\_runner](#output\_vdi\_runner) | Shell runner wrapping Ansible playbook + roles (for custom-image or direct use). |
| <a name="output_vdi_user_credentials"></a> [vdi\_user\_credentials](#output\_vdi\_user\_credentials) | Map of VDI user credentials stored in Secret Manager |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
