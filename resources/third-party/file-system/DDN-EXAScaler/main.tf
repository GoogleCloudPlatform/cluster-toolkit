/**
 * Copyright 2021 Google LLC
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
  named_net_properties = {
    routing = "REGIONAL"
    tier    = "STANDARD"
    name    = var.network_name
    auto    = false
    mtu     = 1500
    new     = false
    nat     = false
  }
  named_subnet_properties = {
    address = var.subnetwork_address
    private = true
    name    = var.subnetwork_name
    new     = false
  }
}

module "ddn_exascaler" {
  source = "github.com/DDNStorage/exascaler-cloud-terraform//gcp?ref=b063430"

  fsname          = var.fsname
  zone            = var.zone
  project         = var.project_id
  admin           = var.admin
  security        = var.security
  service_account = var.service_account
  waiter          = var.waiter
  network         = var.network_name == null ? var.network_properties : local.named_net_properties
  subnetwork      = var.subnetwork_name == null ? var.subnetwork_properties : local.named_subnet_properties
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
