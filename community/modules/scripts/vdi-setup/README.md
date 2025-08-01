## Description

Creates a containerised Guacamole instance. Works with the Rocky, Debian and Ubuntu images shown in blueprint example below.

## Features

- **VDI Tool Support**: Currently supports Guacamole for web-based VDI access
- **User Provisioning**: Supports local user creation with secure password management
- **VNC Integration**: Configures VNC servers for desktop access
- **Secret Manager Integration**: Secure password storage and retrieval for VDI users and webapp admin
- **VDI Monitoring System**: Automatic monitoring and reconfiguration capabilities with password reset flags and deployment status updates
- **Debug Mode**: Comprehensive logging when enabled via the `debug` variable

## Secret Manager Integration

The module integrates with Google Cloud Secret Manager for secure password handling:

- **VDI Users**: Passwords are stored in Secret Manager with the pattern `vdi-user-password-{username}-{deployment_name}`
- **Webapp Admin**: Password is stored in Secret Manager with the pattern `webapp-server-password-{deployment_name}` (always stored, not just when reset is enabled)
- **Password Sources**: Users can specify `secret_name` to fetch existing passwords, provide `password` directly, or let the system generate random passwords
- **Reset Functionality**: Both individual user passwords (`reset_password`) and webapp admin password (`reset_webapp_admin_password`) can be forced to regenerate
- **Database Updates**: Password resets update existing database records without re-initializing the entire database

## VDI Monitoring System

The module includes a monitoring system that:

- **Deployment Status**: Updates instance metadata to reflect deployment state (`available`, `reconfiguring`)
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
        # Several supported image families:
        family: hpc-rocky-linux-8
        project: cloud-hpc-image-public
        #family: debian-11
        #project: debian-cloud
        #family: ubuntu-2204-lts
        #project: ubuntu-os-cloud
        name_prefix: debian
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
