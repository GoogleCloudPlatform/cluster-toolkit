#!/bin/sh
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
set -ex

if [ ! -d "/exports" ]; then # first load, format and mount the disk
    # See https://cloud.google.com/compute/docs/disks/add-persistent-disk
    uuid=$(uuidgen)
    mkfs.ext4 -F -m 0 -U "$uuid" -E lazy_itable_init=0,lazy_journal_init=0,discard /dev/disk/by-id/google-attached_disk

    mkdir /exports
    echo "UUID=$uuid /exports ext4 discard,defaults 0 0" >> /etc/fstab
    mount --target /exports/

    %{ for mount in local_mounts ~}
    mkdir -p /exports${mount}
    chmod 755 /exports${mount}
    echo '/exports${mount} *(rw,sync,no_root_squash)' >> "/etc/exports"    
    %{ endfor ~}
fi

systemctl start nfs-server rpcbind
systemctl enable nfs-server 
exportfs -r
