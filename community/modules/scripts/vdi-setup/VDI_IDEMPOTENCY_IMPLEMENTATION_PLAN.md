# VDI Idempotency Implementation Plan

## Overview
This document outlines the implementation of idempotency for the VDI setup module, ensuring that roles can be safely re-run without causing conflicts or duplicate operations.

## Key Features

### 1. Lock File System
- **Location**: `/opt/vdi-setup/.vdi-lock.yaml`
- **Purpose**: Tracks deployment status, role completion, and configuration changes
- **Structure**: YAML format with deployment metadata and role status

### 2. Role Execution Control
- **Pattern**: Each role checks if it should run before executing tasks
- **Logic**: Roles run if:
  - Lock file doesn't exist (fresh deployment)
  - Role is not marked as completed
  - Deployment configuration has changed
  - Force rerun is enabled

### 3. Setup Status Tracking
- **Status Values**: `configuring`, `available`, `error`
- **Purpose**: Indicates current VDI setup state for external monitoring
- **Usage**: Django app can show "Offline" if VDI is offline (separate logic)

### 4. VM Metadata Integration
- **Content**: Base64-encoded lock file stored in VM metadata
- **Purpose**: External access to VDI status without SSH access
- **Command**: `gcloud compute instances describe <instance> --format="value(metadata.items[vdi-lock-content])" | base64 -d`

### 5. Monitoring Service
- **Service**: `vdi-monitor.service` (systemd)
- **Script**: `/opt/vdi-setup/vdi-monitor.sh`
- **Purpose**: Monitors lock file changes and triggers reconfiguration
- **Bucket Sync**: Pulls latest files from GCS bucket before reconfiguring

## Implementation Details

### Lock File Structure
```yaml
vdi_setup_status:
  deployment_name: "vdi-test-scott"
  deployment_hash: "4d7bbe69940a9da1aae5b3acad02ad140e47a546d53111fb6e9b65e460d11aab"
  lock_version: "1.0"
  created_at: "2025-07-24T10:13:31Z"
  last_updated: "2025-07-24T10:13:31Z"
  force_rerun: false
  setup_status: "configuring"  # configuring, available, error
  
  completed_roles:
    base_os:
      completed: false
    lock_manager:
      completed: true
      completed_at: "2025-07-24T10:13:31Z"
    secret_manager:
      completed: false
    user_provision:
      completed: false
    vnc:
      completed: false
    vdi_tool:
      completed: false
  
  user_secrets_status:
    last_secret_check: "2025-07-24T10:13:31Z"
    user_secrets_hash: "6a6a02d3d09933ca282ae2b02336343c9d73701e45bd81475fbd92dcf9d2f3b6"
    users_updated:
    - alice
    - bob
  
  user_management:
    current_users:
    - alice
    - bob
    previous_users:
    - alice
    - bob
    removed_users: []
```

### Role Execution Pattern
Each role follows this pattern:
```yaml
# Check if this role should run
- name: Check if <role_name> role should run
  ansible.builtin.import_role:
    name: lock_manager
    tasks_from: check_lock
  vars:
    current_role: "<role_name>"

# Skip all tasks if role should not run
- name: Skip <role_name> tasks if role should not run
  ansible.builtin.debug:
    msg: "Skipping <role_name> role - already completed or not needed"
  when: not role_should_run

# Role-specific tasks with conditional execution
- name: Role task
  ansible.builtin.task:
    # task details
  when: role_should_run

# Mark role as completed
- name: Mark <role_name> role as completed
  ansible.builtin.import_role:
    name: lock_manager
    tasks_from: create_lock
  vars:
    current_role: "<role_name>"
    role_completed: true
  when: role_should_run
```

### Change Detection
- **Deployment Hash**: SHA-256 hash of deployment configuration
- **User Secrets Hash**: SHA-256 hash of user configuration (blueprint data only)
- **Role Completion**: Individual tracking of each role's completion status

### Secret Manager Integration
- **Blueprint-based Detection**: User secrets hash is calculated from blueprint data (usernames, ports, secret_names, etc.)
- **Password Changes**: Changes to passwords in Secret Manager are **not automatically detected**
- **Manual Re-initialization**: To apply password changes, use `force_rerun: true` in lock file
- **Secret ID Changes**: Changes to `secret_name` references are automatically detected

### Monitoring Service
- **Trigger**: Lock file modification detected
- **Action**: Sync files from GCS bucket and run Ansible reconfiguration
- **Logging**: Detailed logs in `/var/log/vdi-monitor.log`

## Benefits

1. **Idempotency**: Safe to re-run deployment without conflicts
2. **Efficiency**: Skip completed roles to reduce deployment time
3. **Monitoring**: External visibility into VDI status
4. **Automation**: Automatic reconfiguration on configuration changes
5. **Reliability**: Proper error handling and status tracking

## Usage

### Manual Status Check
```bash
# Check on-disk lock file
cat /opt/vdi-setup/.vdi-lock.yaml

# Check VM metadata
gcloud compute instances describe <instance> --zone=<zone> --format="value(metadata.items[vdi-lock-content])" | base64 -d
```

### Force Re-run
```bash
# Edit lock file to force re-run
sed -i 's/force_rerun: false/force_rerun: true/' /opt/vdi-setup/.vdi-lock.yaml
```

### Handle Secret Manager Password Changes
```bash
# To apply password changes from Secret Manager:
# 1. Update the password in Secret Manager
# 2. Force re-run to apply changes
sed -i 's/force_rerun: false/force_rerun: true/' /opt/vdi-setup/.vdi-lock.yaml

# 3. Run the playbook again
cd /tmp/vdi && ansible-playbook install.yaml --extra-vars @vars.yaml
```

### Monitor Service
```bash
# Check service status
systemctl status vdi-monitor

# View logs
tail -f /var/log/vdi-monitor.log
``` 