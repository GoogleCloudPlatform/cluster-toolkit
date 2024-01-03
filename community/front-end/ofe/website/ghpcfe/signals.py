# Copyright 2022 Google LLC
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

"""Signal handlers for model state"""

from django.db.models.signals import pre_save, post_delete, post_save
from django.dispatch import receiver
from .models import Cluster, VirtualNetwork

# Pylint misses the sender decorator behaviour here
#pylint: disable=unused-argument

@receiver(post_save, sender=VirtualNetwork)
def sync_vnet_subnet_state(sender, **kwargs):
    vpc = kwargs["instance"]
    for sn in vpc.subnets.all():
        sn.cloud_state = vpc.cloud_state
        sn.save()


@receiver(post_delete, sender=Cluster)
def delete_cluster_extras(sender, **kwargs):
    cluster = kwargs["instance"]
    if cluster.shared_fs:
        cluster.shared_fs.delete()
    if cluster.controller_node:
        cluster.controller_node.delete()


@receiver(pre_save, sender=Cluster)
def sync_cluster_fs_ip(sender, **kwargs):
    cluster = kwargs["instance"]
    if cluster.subnet:
        cluster.cloud_region = cluster.subnet.cloud_region
    if cluster.shared_fs:
        cluster.shared_fs.cloud_id = cluster.cloud_id
        cluster.shared_fs.cloud_state = cluster.cloud_state
        cluster.shared_fs.cloud_region = cluster.cloud_region
        cluster.shared_fs.cloud_zone = cluster.cloud_zone
        cluster.shared_fs.cloud_credential = cluster.cloud_credential
        cluster.shared_fs.name = f"{cluster.name}-sharedfs"
        cluster.shared_fs.internal_name = f"{cluster.name} SharedFS"
        cluster.shared_fs.subnet = cluster.subnet
        if cluster.controller_node:
            cluster.shared_fs.hostname_or_ip = (
                cluster.controller_node.internal_ip
            )
        cluster.shared_fs.save()
