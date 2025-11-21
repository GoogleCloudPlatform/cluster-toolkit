# Lock Manager Role

The `lock_manager` role is the central coordination mechanism for the VDI setup module. It manages deployment state, change detection, and ensures idempotent role execution.

## Overview

The lock manager uses a hash-based change detection system to determine when roles should run, preventing unnecessary re-execution while ensuring changes are properly applied.

## Key Features

### 1. **Hash-Based Change Detection**

- **Deployment Hash**: Tracks changes to deployment configuration (ports, tools, etc.)
- **User Hash**: Tracks changes to user configurations (additions, removals, password resets)
- **Smart Detection**: Only re-runs roles when relevant changes are detected

### 2. **Centralized Variable Management**

- **User Management Variables**: Calculates and propagates user change information to all roles
- **Port Change Detection**: Detects and tracks VNC port changes for existing users
- **Password Reset Tracking**: Identifies users requiring password resets

### 3. **Targeted Updates**

- **Database-Only Updates**: Performs user additions/deletions/password resets without container recreation
- **Port Change Handling**: Updates VNC ports and Guacamole connections without full re-initialization
- **Webapp Admin Password Reset**: Updates admin password without container recreation

### 4. **Deployment Status Management**

- **Lock File**: `.vdi-lock.yaml` tracks deployment state and completed roles
- **Status Tracking**: `available`, `configuring`, `reconfiguring`, `failed` states
- **Role Completion**: Tracks which roles have completed successfully

## Variable Propagation

### **Variables Calculated by Lock Manager:**

| Variable | Description | Usage |
|----------|-------------|-------|
| `new_users` | Users added since last deployment | `vdi_tool`, `user_provision`, `vnc` |
| `removed_users` | Users removed since last deployment | `vdi_tool`, `user_provision`, `vnc` |
| `users_needing_reset` | Users with `reset_password: true` | `vdi_tool`, `secret_manager` |
| `users_with_port_changes` | Users whose VNC ports changed | `vdi_tool`, `vnc` |
| `webapp_port_changed` | Whether webapp port changed | `vdi_tool` |
| `users_changed` | Any user-related changes detected | `vdi_tool` |
| `db_needs_init` | Whether database needs re-initialization | `vdi_tool` |
| `vdi_user_group` | Current VDI user group for group management | `user_provision` |

### **Variable Propagation Flow:**

1. **Lock Manager Calculation**: Variables calculated once during initial `lock_manager` run
2. **Role Import**: Other roles import `lock_manager` and receive these variables
3. **Defensive Programming**: Variables preserved with `default()` filters, not overwritten
4. **Role Usage**: Each role uses only the variables it needs

## Data Flows

### **Initial Deployment:**

```mermaid
1. lock_manager → Calculates all variables (empty for fresh deployment)
2. base_os → OS setup
3. secret_manager → Password generation and Secret Manager setup
4. user_provision → Local user creation
5. vnc → VNC service setup
6. vdi_tool → Guacamole deployment
7. vdi_monitor → Monitoring service setup
```

### **Reconfiguration (User Changes):**

```mermaid
1. lock_manager → Detects changes, calculates user management variables
2. user_provision → Handles user additions/removals
3. vnc → Handles VNC port changes and user removal
4. vdi_tool → Handles database updates (targeted, no container recreation)
5. vdi_monitor → No changes needed
```

### **Reconfiguration (Port Changes):**

```mermaid
1. lock_manager → Detects port changes
2. vnc → Stops VNC services for users with port changes
3. vdi_tool → Updates Guacamole connection ports in database
4. user_provision → Updates local user configurations
```

## Lock File Structure

```yaml
vdi_setup_status:
  completed_roles:
    base_os: true
    secret_manager: true
    user_provision: true
    vdi_monitor: true
    vdi_tool: true
    vnc: true
  created_at: '2025-08-07T16:20:26Z'
  current_users: ['alice', 'bob', 'charlie', 'david']
  deployment_hash: 'abc123...'
  deployment_name: 'vdi-test-scott'
  force_rerun: false
  last_updated: '2025-08-07T16:28:42Z'
  lock_version: '1.0'
  setup_status: 'available'
  user_hash: 'def456...'
  user_ports:
    alice: 5901
    bob: 5902
    charlie: 5903
    david: 5904
  webapp_port: 8081
  vdi_user_group: vdiusers
```

## Role Execution Logic

### **When Roles Run:**
- **Fresh Deployment**: All roles run regardless of hash
- **Hash Mismatch**: Only roles with changed hashes run
- **Force Rerun**: All roles run when `force_rerun: true`

### **Role Completion:**
- Roles mark themselves as completed in lock file
- Lock file tracks completion status per role
- Deployment status updated based on all role completions

## Error Handling

### **Failed Deployments:**
- VDI monitor sets status to `"failed"` when Ansible exits with non-zero code
- Lock file preserves failed state for debugging
- Valid blueprint can be re-applied to fix `failed` deployments

### **Variable Safety:**
- Defensive programming ensures all variables have safe defaults
- `default()` filters prevent undefined variable errors
- Variables preserved across role imports

## Debug Output

When `debug: true` is set, the lock manager provides detailed information about:
- Variable calculations and propagation
- Change detection results
- Hash comparisons
- Role execution decisions
- Port change detection details
