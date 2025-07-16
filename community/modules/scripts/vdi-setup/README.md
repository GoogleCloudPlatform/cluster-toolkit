# Copyright 2025 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

## Description

Mostly tested with Rocky. Working on Debian and Ubuntu images (commented in blueprint example below).
Distro / VDI 'flavour' variations are handled in the 'base_os' role for the most part.

### Service Startup and Health Checking

The VDI module includes basic health checks to ensure services start properly:

### Secret Manager Integration

The VDI module supports flexible Secret Manager integration:

- **Default Behavior**: Secrets are stored in the deployment project
- **Cross-Project Secrets**: Use `secret_project` to specify a different GCP project for user secrets
- **Automatic Password Generation**: If no `secret_name` is provided, random passwords are generated and stored
- **Existing Secret Retrieval**: Provide `secret_name` to use existing secrets from Secret Manager

- **Database Health Check**: Waits for PostgreSQL to be ready on port 5432 (60 second timeout)
- **Guacamole Web Interface**: Checks HTTP endpoint availability
- **API Verification**: Verifies Guacamole admin login functionality
- **VNC Service Restart**: Systemd services configured with automatic restart on failure

## Outputs

The module provides the following outputs:

- `guacamole_url`: The URL to access the Guacamole web interface (null until VM is created)
- `guacamole_admin_username`: The admin username for Guacamole (default: "guacadmin")
- `guacamole_admin_password_secret`: The name of the Secret Manager secret containing the Guacamole admin password
  - `vdi_user_credentials`: A map of VDI user credentials stored in Secret Manager, including:
  - `username`: The VDI user's username
  - `port`: The VNC port assigned to the user
  - `secret_name`: The name of the Secret Manager secret containing the user's password
  - `secret_project`: The GCP project where the user's secret is stored (defaults to deployment project) 
- `vdi_instance_ip`: The IP address of the VDI instance (null until VM is created)
- `vdi_instance_name`: The name of the VDI instance (null until VM is created)

## Basic Example (Guacamole)

```
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
        ranges: ["0.0.0.0/0"] # or rev proxy address?
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
        # Several spported image families:
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
     - Note: It is not advisable to serve Guacamole directly from a public IP in production environments. Consider serving your VDI from behind a reverse proxy, load balancer, or tunnel to it directly over IAP (see below).

   - Admin credentials:
     - Username: `guacadmin`
     - Password: Retrieve the `webapp-server...` secret from Secret Manager

2. **User VDI Access**:
   - Each user's credentials are stored in Secret Manager
   - Secret names follow the pattern: `vdi-user-password-{username}-{deployment_name}` for auto-generated passwords
   - Or use the custom secret name specified in the `vdi_users` configuration

3. **IAP Tunnel** (for development/testing):
   ```
   gcloud compute start-iap-tunnel vdi-test-0 8080 \
       --local-host-port=localhost:8080 \
       --zone=us-central1-a
   ```
   - Guacamole will then be accessible from http://localhost:8080/guacamole/
