/**
 * Copyright 2022 Google LLC
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
locals {

  network_id = var.network_self_link != null ? regex("https://www.googleapis.com/compute/v\\d/(.*)", var.network_self_link)[0] : null
  named_net = {
    routing = "REGIONAL"
    tier    = "STANDARD"
    id      = local.network_id
    auto    = false
    mtu     = 1500
    new     = false
    nat     = false
  }

  subnetwork_id = var.subnetwork_self_link != null ? regex("https://www.googleapis.com/compute/v\\d/(.*)", var.subnetwork_self_link)[0] : null
  named_subnet = {
    address = var.subnetwork_address
    private = true
    id      = local.subnetwork_id
    new     = false
  }
}

module "ddn_exascaler" {
  source          = "github.com/DDNStorage/exascaler-cloud-terraform//gcp?ref=78deadb"
  fsname          = var.fsname
  zone            = var.zone
  project         = var.project_id
  prefix          = var.prefix
  labels          = var.labels
  security        = var.security
  service_account = var.service_account
  waiter          = var.waiter
  network         = var.network_properties == null ? local.named_net : var.network_properties
  subnetwork      = var.subnetwork_properties == null ? local.named_subnet : var.subnetwork_properties
  boot            = var.boot
  image           = var.image
  mgs             = var.mgs
  mgt             = var.mgt
  mnt             = var.mnt
  mds             = var.mds
  mdt             = var.mdt
  oss             = var.oss
  ost             = var.ost
  cls             = var.cls
  clt             = var.clt
}
