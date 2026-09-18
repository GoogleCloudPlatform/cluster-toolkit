# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

locals {
  install_root    = trimsuffix(var.install_root, "/")
  slurm_auth_mode = lower(trimspace(var.slurm_auth_mode))
  vnc_backend     = lower(trimspace(var.vnc_backend))
  identity_mode   = lower(trimspace(var.identity_mode))


  broker_dir          = "/etc/ghpc-desktop-broker"
  broker_config_path  = "${local.broker_dir}/config.json"
  broker_app_dir      = "${local.install_root}/desktop-broker"
  broker_package_dir  = "${local.broker_app_dir}/desktop_broker"
  broker_state_dir    = "/var/lib/ghpc-remote-desktop"
  broker_log_dir      = "/var/log/ghpc-remote-desktop"
  broker_runtime_dir  = "/run/ghpc-remote-desktop"
  broker_service_path = "/etc/systemd/system/ghpc-desktop-broker.service"
  broker_venv_dir     = "${local.install_root}/desktop-broker-venv"
  broker_python_path  = "${local.broker_venv_dir}/bin/python3"
  broker_pip_path     = "${local.broker_venv_dir}/bin/pip"

  novnc_dir = "${local.install_root}/novnc"


  mount_retry_attempts = 30
  mount_retry_seconds  = 10

  read_secret_sh = templatefile("${path.module}/templates/read-secret.sh.tftpl", {
    secret_project_id = var.secret_project_id == null ? "" : var.secret_project_id
  })

  # Nothing else on the host has to be up before the broker.
  requires_units = ""
}

# ---------------------------------------------------------------- XFCE + VNC
locals {
  backend_installers = {
    tigervnc = {
      dnf = file("${path.module}/templates/installers/tigervnc-dnf.sh")
      apt = file("${path.module}/templates/installers/tigervnc-apt.sh")
    }
    turbovnc = {
      dnf = file("${path.module}/templates/installers/turbovnc-dnf.sh")
      apt = file("${path.module}/templates/installers/turbovnc-apt.sh")
    }
  }

  # Only TurboVNC needs VirtualGL: TigerVNC's Xvnc offloads GL itself given
  # "-rendernode", so installing VirtualGL alongside it would be dead weight.
  install_virtualgl = var.enable_gpu_acceleration && local.vnc_backend == "turbovnc"

  virtualgl_dnf = local.install_virtualgl ? file("${path.module}/templates/installers/virtualgl-dnf.sh") : ""
  virtualgl_apt = local.install_virtualgl ? file("${path.module}/templates/installers/virtualgl-apt.sh") : ""

  desktop_runtime_runners = [
    {
      type        = "shell"
      destination = "desktop-runtime-setup.sh"
      content = templatefile("${path.module}/templates/desktop-runtime-setup.sh.tftpl", {
        vnc_backend        = local.vnc_backend
        dnf_install_script = join("\n", compact([local.backend_installers[local.vnc_backend].dnf, local.virtualgl_dnf]))
        apt_install_script = join("\n", compact([local.backend_installers[local.vnc_backend].apt, local.virtualgl_apt]))
      })
    }
  ]
}

# ------------------------------------------------------- storage and Slurm
locals {
  user_startup_script_runners = var.startup_script == null ? [] : [
    {
      type        = "shell"
      content     = var.startup_script
      destination = "user_startup_script.sh"
    }
  ]

  network_storage_client_install_runners = [
    for index, ns in var.network_storage : merge(ns.client_install_runner, {
      destination = "${index}-${ns.client_install_runner.destination}"
    }) if ns.client_install_runner != null
  ]

  network_storage_mount_runners = [
    for index, ns in var.network_storage : {
      type        = "shell"
      destination = "${index}-${ns.mount_runner.destination}"
      content = templatefile("${path.module}/templates/retry-mount-runner.sh.tftpl", {
        runner_content_b64 = base64encode(
          can(ns.mount_runner.content) ? ns.mount_runner.content : file(ns.mount_runner.source)
        )
        runner_args     = lookup(ns.mount_runner, "args", "")
        retry_attempts  = local.mount_retry_attempts
        retry_seconds   = local.mount_retry_seconds
        continue_on_err = contains([for option in split(",", ns.mount_options) : trimspace(option)], "nofail")
      })
    } if ns.mount_runner != null
  ]

  slurm_auth_runners = contains(["native", "auto"], local.slurm_auth_mode) ? [
    {
      type        = "shell"
      destination = "slurm-auth-mode.sh"
      content     = <<-EOT
        #!/bin/bash
        set -euo pipefail

        slurm_auth_mode=${jsonencode(local.slurm_auth_mode)}

        detect_auth_type() {
          local config_path
          for config_path in /usr/local/etc/slurm.conf /etc/slurm/slurm.conf; do
            if [[ -f "$config_path" ]]; then
              awk -F= '
                BEGIN { IGNORECASE = 1 }
                $1 ~ /^[[:space:]]*AuthType[[:space:]]*$/ {
                  gsub(/[[:space:]]/, "", $2)
                  print tolower($2)
                  exit
                }
              ' "$config_path"
              return 0
            fi
          done
          return 1
        }

        auth_type=""
        case "$slurm_auth_mode" in
          native)
            auth_type="auth/slurm"
            ;;
          auto)
            auth_type="$(detect_auth_type || true)"
            ;;
          *)
            exit 0
            ;;
        esac

        if [[ "$auth_type" != "auth/slurm" ]]; then
          exit 0
        fi

        if systemctl list-unit-files munge.service >/dev/null 2>&1; then
          systemctl disable --now munge.service || true
          systemctl reset-failed munge.service || true
        fi
      EOT
    },
  ] : []
}

# ------------------------------------------------------------ the front end
locals {
  novnc_runners = [
    {
      type        = "shell"
      destination = "novnc-install.sh"
      content = templatefile("${path.module}/templates/novnc-install.sh.tftpl", {
        install_root  = local.install_root
        novnc_dir     = local.novnc_dir
        novnc_version = var.novnc_version
      })
    }
  ]

}

# ------------------------------------------------------------- the broker
locals {
  broker_package_files = fileset("${path.module}/files/desktop_broker", "**/*.py")

  # Written by a single runner rather than one "data" runner per file.
  # modules/scripts/startup-script keys its staged objects on
  # basename(destination), and a Python package has an __init__.py in every
  # subpackage, so per-file runners make that key collide and Terraform rejects
  # the plan. Embedding the files base64-encoded in one script sidesteps the
  # key entirely and needs no extra provider.
  broker_package_runners = [
    {
      type        = "shell"
      destination = "broker-package.sh"
      content = templatefile("${path.module}/templates/broker-package.sh.tftpl", {
        package_dir = local.broker_package_dir
        files = {
          for relative_path in local.broker_package_files :
          relative_path => base64encode(file("${path.module}/files/desktop_broker/${relative_path}"))
        }
      })
    }
  ]

  # One flat map rather than a conditional merge: Terraform cannot unify two
  # ternary branches with different attributes, and the broker ignores keys that
  # do not apply to its front end.
  broker_config = {
    base_display_number          = var.vnc_display_number
    gpu_acceleration             = var.enable_gpu_acceleration
    identity_mode                = local.identity_mode
    listen_host                  = "0.0.0.0"
    listen_port                  = var.broker_listen_port
    log_dir                      = local.broker_log_dir
    max_user_sessions            = var.max_user_sessions
    runtime_dir                  = local.broker_runtime_dir
    session_idle_timeout_seconds = var.session_idle_timeout_seconds
    session_resolution           = var.session_resolution
    state_dir                    = local.broker_state_dir
    vnc_backend                  = local.vnc_backend

    novnc_dir = local.novnc_dir

    # proxy_secret and json_secret_key are deliberately absent: the install
    # runner fetches them on the instance and merges them in before the broker
    # starts. Putting them here would place them in Terraform state and in the
    # startup script staged in Cloud Storage.
  }

  broker_runners = concat(
    local.broker_package_runners,
    [
      {
        type        = "data"
        content     = jsonencode(local.broker_config)
        destination = local.broker_config_path
      },
      {
        type        = "shell"
        destination = "broker-install.sh"
        content = templatefile("${path.module}/templates/broker-install.sh.tftpl", {
          install_root        = local.install_root
          broker_dir          = local.broker_dir
          broker_config_path  = local.broker_config_path
          broker_app_dir      = local.broker_app_dir
          broker_state_dir    = local.broker_state_dir
          broker_log_dir      = local.broker_log_dir
          broker_runtime_dir  = local.broker_runtime_dir
          broker_service_path = local.broker_service_path
          broker_venv_dir     = local.broker_venv_dir
          broker_python_path  = local.broker_python_path
          broker_pip_path     = local.broker_pip_path
          broker_listen_port  = var.broker_listen_port
          requires_units      = local.requires_units
          read_secret_sh      = local.read_secret_sh

          proxy_secret_id      = var.proxy_secret_id == null ? "" : var.proxy_secret_id
          proxy_secret_version = var.proxy_secret_version

          # Literals travel base64-encoded. A secret is arbitrary bytes, and
          # embedding one in a double-quoted shell assignment would let a value
          # containing $(...) or a backtick execute during install.
          proxy_secret_b64 = var.proxy_secret == null ? "" : base64encode(var.proxy_secret)
        })
      },
    ],
  )
}

# ------------------------------------------------------ service discovery
locals {
  endpoint_publish_runners = var.desktop_endpoint_dir == null ? [] : [
    {
      type        = "shell"
      destination = "publish-desktop-endpoint.sh"
      content     = <<-EOT
        #!/bin/bash
        set -euo pipefail

        endpoint_dir=${jsonencode(var.desktop_endpoint_dir)}
        endpoint_name=${jsonencode(var.desktop_endpoint_name)}
        endpoint_port=${jsonencode(var.broker_listen_port)}

        install -d -m 0755 "$endpoint_dir"

        instance_name=$(curl -fsS -H "Metadata-Flavor: Google" \
          "http://metadata.google.internal/computeMetadata/v1/instance/name")
        instance_ip=$(curl -fsS -H "Metadata-Flavor: Google" \
          "http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ip")

        cat > "$${endpoint_dir}/$${endpoint_name}.env" <<EOF_ENDPOINT
        DESKTOP_NAME=$${instance_name}
        DESKTOP_HOST=$${instance_ip}
        DESKTOP_PORT=$${endpoint_port}
        EOF_ENDPOINT
      EOT
    },
  ]

  startup_runners = concat(
    local.network_storage_client_install_runners,
    local.network_storage_mount_runners,
    local.user_startup_script_runners,
    local.slurm_auth_runners,
    local.desktop_runtime_runners,
    local.novnc_runners,
    local.broker_runners,
    local.endpoint_publish_runners,
  )
}

resource "terraform_data" "input_validation" {
  input = {
    identity_mode = local.identity_mode
  }

  lifecycle {
    precondition {
      condition     = (var.proxy_secret == null) != (var.proxy_secret_id == null)
      error_message = "Set exactly one of proxy_secret or proxy_secret_id. Prefer proxy_secret_id, which keeps the value out of Terraform state and out of the startup script staged in Cloud Storage; use proxy_secret where the caller already holds the plaintext anyway."
    }






    precondition {
      condition     = var.vnc_display_number >= 1
      error_message = "vnc_display_number must be at least 1. Display :0 is reserved for a physical console."
    }



  }
}
