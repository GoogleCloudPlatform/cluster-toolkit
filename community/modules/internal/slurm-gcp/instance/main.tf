/**
 * Copyright (C) SchedMD LLC.
 * Copyright 2018 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

##########
# LOCALS #
##########

locals {
  num_instances = length(var.static_ips) == 0 ? var.num_instances : length(var.static_ips)

  # local.static_ips is the same as var.static_ips with a dummy element appended
  # at the end of the list to work around "list does not have any elements so cannot
  # determine type" error when var.static_ips is empty
  static_ips = concat(var.static_ips, ["NOT_AN_IP"])

  network_interfaces = [for index in range(local.num_instances) :
    concat([
      {
        access_config      = var.access_config
        alias_ip_range     = []
        ipv6_access_config = []
        network            = var.network
        network_ip         = length(var.static_ips) == 0 ? "" : element(local.static_ips, index)
        nic_type           = null
        queue_count        = null
        stack_type         = null
        subnetwork         = var.subnetwork
        subnetwork_project = var.subnetwork_project
      }
      ],
      var.additional_networks
    )
  ]
}

################
# DATA SOURCES #
################

data "google_compute_zones" "available" {
  project = var.project_id
  region  = var.region
}

data "google_compute_instance_template" "base" {
  project = var.project_id
  name    = var.instance_template
}

#############
# INSTANCES #
#############
resource "null_resource" "replace_trigger" {
  triggers = {
    trigger = var.replace_trigger
  }
}

resource "google_compute_instance_from_template" "slurm_instance" {
  count   = local.num_instances
  name    = format("%s-%s", var.hostname, format("%03d", count.index + 1))
  project = var.project_id
  zone    = var.zone == null ? data.google_compute_zones.available.names[count.index % length(data.google_compute_zones.available.names)] : var.zone

  allow_stopping_for_update = true

  dynamic "network_interface" {
    for_each = local.network_interfaces[count.index]
    iterator = nic
    content {
      dynamic "access_config" {
        for_each = nic.value.access_config
        content {
          nat_ip       = access_config.value.nat_ip
          network_tier = access_config.value.network_tier
        }
      }
      dynamic "alias_ip_range" {
        for_each = nic.value.alias_ip_range
        content {
          ip_cidr_range         = alias_ip_range.value.ip_cidr_range
          subnetwork_range_name = alias_ip_range.value.subnetwork_range_name
        }
      }
      dynamic "ipv6_access_config" {
        for_each = nic.value.ipv6_access_config
        iterator = access_config
        content {
          network_tier = access_config.value.network_tier
        }
      }
      network            = nic.value.network
      network_ip         = nic.value.network_ip
      nic_type           = nic.value.nic_type
      queue_count        = nic.value.queue_count
      subnetwork         = nic.value.subnetwork
      subnetwork_project = nic.value.subnetwork_project
    }
  }

  source_instance_template = data.google_compute_instance_template.base.self_link

  lifecycle {
    replace_triggered_by = [null_resource.replace_trigger.id]
  }
}
