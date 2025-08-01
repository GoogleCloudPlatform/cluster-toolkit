# VDI Monitor Role

## Overview

The `vdi_monitor` role sets up and configures the VDI Configuration Monitor system. This role is responsible for:

- Installing the modular VDI monitoring scripts
- Configuring the systemd service for automatic monitoring
- Setting up file change detection
- Enabling automatic VDI reconfiguration

## Features

### 🔍 Change Detection
- **File Monitoring**: Detects changes to `vars.yaml`, `install.yaml`, and `roles.tar.gz` in GCS bucket
- **Password Reset Detection**: Detects `reset_password: true` flags in configuration
- **Immediate Initialization**: Refreshes file tracking on startup to capture current bucket state

### ⚡ Automatic Reconfiguration
- **Immediate Action**: Triggers reconfiguration as soon as changes are detected
- **Cooldown**: Prevents excessive reconfigurations with a 1-minute cooldown period
- **Idempotent**: Safe to run multiple times without side effects
- **Smart Container Management**: User changes handled via SQL updates, full container recreation only when necessary

### 🛠️ Modular Architecture
- **Config Module**: Constants, logging, and initialization
- **File Monitor**: GCS file detection and synchronization
- **Change Detector**: Change detection logic and reconfiguration
- **Test Utils**: Comprehensive testing and diagnostics

## Role Structure

```
vdi_monitor/
├── defaults/
│   └── main.yaml          # Default variables
├── tasks/
│   └── main.yaml          # Main role tasks
├── templates/
│   ├── vdi-monitor.sh.j2              # Main orchestrator script
│   ├── vdi-monitor-config.sh.j2       # Configuration module
│   ├── vdi-monitor-file.sh.j2         # File monitoring module
│   ├── vdi-monitor-detector.sh.j2     # Change detection module
│   ├── vdi-monitor-test.sh.j2         # Testing module
│   └── vdi-monitor.service.j2         # Systemd service
├── meta/
│   └── main.yaml          # Role metadata
└── README.md              # This file
```

## Variables

### Required Variables
None - all variables have sensible defaults.

### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `vdi_monitor_check_interval` | `60` | Monitoring check interval in seconds |
| `vdi_monitor_reconfig_cooldown` | `60` | Reconfiguration cooldown in seconds (1 minute) |
| `vdi_monitor_log_file` | `"/var/log/vdi-monitor.log"` | Monitor log file |
| `vdi_monitor_ansible_log_file` | `"/var/log/ansible-vdi-reconfig.log"` | Ansible reconfig log |

## Usage

### Basic Usage
The role is automatically included in the VDI setup playbook:

```yaml
# In your VDI blueprint
- id: vdi_setup
  source: modules/scripts/vdi-setup
  # ... other configuration
```

### Manual Role Execution
```bash
# Run the role manually
ansible-playbook -i localhost, --connection=local \
  -e "role=vdi_monitor" \
  /opt/vdi-setup/install.yaml
```

## Monitoring Commands

### Test Mode
```bash
# Run comprehensive tests
sudo /usr/local/bin/vdi-monitor.sh --test
```

### Diagnostics
```bash
# Run system diagnostics
sudo /usr/local/bin/vdi-monitor.sh --diagnostics
```

### Help
```bash
# Show help and usage
sudo /usr/local/bin/vdi-monitor.sh --help
```

## Service Management

### Check Service Status
```bash
systemctl status vdi-monitor
```

### View Logs
```bash
# Monitor logs
tail -f /var/log/vdi-monitor.log

# Ansible reconfiguration logs
tail -f /var/log/ansible-vdi-reconfig.log
```

### Manual Service Control
```bash
# Start service
sudo systemctl start vdi-monitor

# Stop service
sudo systemctl stop vdi-monitor

# Restart service
sudo systemctl restart vdi-monitor

# Enable/disable service
sudo systemctl enable vdi-monitor
sudo systemctl disable vdi-monitor
```

## Dependencies

- `lock_manager` role (for deployment locking)
- Google Cloud SDK (`gcloud`, `gsutil`)
- Ansible (`ansible-playbook`)
- Systemd (for service management)

## Files Created

### Scripts
- `/usr/local/bin/vdi-monitor.sh` - Main monitoring script
- `/opt/vdi-setup/vdi-monitor-lib/` - Module directory
  - `config.sh` - Configuration module
  - `file-monitor.sh` - File monitoring module
  - `change-detector.sh` - Change detection module
  - `test-utils.sh` - Testing module

### Services
- `/etc/systemd/system/vdi-monitor.service` - Systemd service

### Logs
- `/var/log/vdi-monitor.log` - Monitor activity log
- `/var/log/ansible-vdi-reconfig.log` - Reconfiguration log

### State Files
- `/opt/vdi-setup/.current_*_file` - File tracking state
- `/opt/vdi-setup/.pending_changes` - Pending change details

## How It Works

### Change Detection Process
1. **File Monitoring**: Every 60 seconds, checks for changes in GCS bucket files
2. **Immediate Response**: When changes are detected, triggers reconfiguration immediately
3. **File Sync**: Downloads updated files from GCS bucket
4. **Ansible Execution**: Runs the VDI setup playbook with updated configuration
5. **Cooldown**: Prevents reconfigurations for 1 minute after successful execution

### Password Reset Workflow
1. **Blueprint Update**: User adds `reset_password: true` to a user in blueprint
2. **Terraform Deployment**: New `vars.yaml` is uploaded to GCS with new file suffix
3. **Change Detection**: VDI monitor detects the new file suffix
4. **Reconfiguration**: Ansible roles re-run with `reset_password: true` flag
5. **Password Update**: New password is generated and stored in Secret Manager
6. **Container Preservation**: User changes are handled via SQL updates without recreating containers

## Troubleshooting

### Common Issues

1. **Service won't start**
   - Check logs: `journalctl -u vdi-monitor`
   - Verify permissions: `ls -la /usr/local/bin/vdi-monitor.sh`
   - Check dependencies: `which gcloud gsutil ansible-playbook`

2. **No changes detected**
   - Run test mode: `sudo /usr/local/bin/vdi-monitor.sh --test`
   - Check bucket access: `gsutil ls gs://your-bucket-name/`
   - Verify file tracking: Check `.current_*_file` files in `/opt/vdi-setup/`
   - Check bucket name retrieval: Look for "DEBUG: Bucket name" in logs

3. **Ansible not running after change detection**
   - Check Ansible logs: `tail -f /var/log/ansible-vdi-reconfig.log`
   - Verify files exist: `ls -la /opt/vdi-setup/install.yaml /opt/vdi-setup/vars.yaml`
   - Run diagnostics: `sudo /usr/local/bin/vdi-monitor.sh --diagnostics`
