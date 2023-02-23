#!/usr/bin/python
# -*- coding: utf-8 -*-

# Copyright 2018 Google Inc. All Rights Reserved.
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

# Script for resizing managed instance group (MIG) cluster size based
# on the number of jobs in the Condor Queue.

from absl import app
from absl import flags
from pprint import pprint
from googleapiclient import discovery
from oauth2client.client import GoogleCredentials

import os
import math
import argparse
import datetime
import htcondor
import classad

parser = argparse.ArgumentParser()
parser.add_argument("--p", required=True, help="Project id", type=str)
parser.add_argument(
    "--z",
    required=True,
    help="Name of GCP zone where the managed instance group is located",
    type=str,
)
parser.add_argument(
    "--r",
    required=True,
    help="Name of GCP region where the managed instance group is located",
    type=str,
)
parser.add_argument(
    "--mz",
    required=False,
    help="Enabled multizone (regional) managed instance group",
    action="store_true",
)
parser.add_argument(
    "--g", required=True, help="Name of the managed instance group", type=str
)
parser.add_argument(
    "--c", required=True, help="Maximum number of compute instances", type=int
)
parser.add_argument(
    "--v",
    default=0,
    help="Increase output verbosity. 1-show basic debug info. 2-show detail debug info",
    type=int,
    choices=[0, 1, 2],
)
parser.add_argument(
    "--d",
    default=0,
    help="Dry Run, default=0, if 1, then no scaling actions",
    type=int,
    choices=[0, 1],
)

args = parser.parse_args()

class AutoScaler:
    def __init__(self, multizone=False):

        self.multizone = multizone
        # Obtain credentials
        self.credentials = GoogleCredentials.get_application_default()
        self.service = discovery.build("compute", "v1", credentials=self.credentials)

        if self.multizone:
            self.instanceGroupManagers = self.service.regionInstanceGroupManagers()
        else:
            self.instanceGroupManagers = self.service.instanceGroupManagers()

    # Remove specified instance from MIG and decrease MIG size
    def deleteFromMig(self, instance, zone):
        instanceUrl = "https://www.googleapis.com/compute/v1/projects/" + self.project
        instanceUrl += "/zones/" + zone
        instanceUrl += "/instances/" + instance

        instances_to_delete = {"instances": [instanceUrl]}

        requestDelInstance = self.instanceGroupManagers.deleteInstances(
            project=self.project,
            **self.zoneargs,
            instanceGroupManager=self.instance_group_manager,
            body=instances_to_delete,
        )

        # execute if not a dry-run
        if not self.dryrun:
            response = requestDelInstance.execute()
            if self.debug > 0:
                print("Request to delete instance " + instance)
                pprint(response)
            return response
        return "Dry Run"

    def getInstanceTemplateInfo(self):
        requestTemplateName = self.instanceGroupManagers.get(
            project=self.project,
            **self.zoneargs,
            instanceGroupManager=self.instance_group_manager,
            fields="instanceTemplate",
        )
        responseTemplateName = requestTemplateName.execute()
        template_name = ""

        if self.debug > 1:
            print("Request for the template name")
            pprint(responseTemplateName)

        if len(responseTemplateName) > 0:
            template_url = responseTemplateName.get("instanceTemplate")
            template_url_partitioned = template_url.split("/")
            template_name = template_url_partitioned[len(template_url_partitioned) - 1]

        requestInstanceTemplate = self.service.instanceTemplates().get(
            project=self.project, instanceTemplate=template_name, fields="properties"
        )
        responseInstanceTemplateInfo = requestInstanceTemplate.execute()

        if self.debug > 1:
            print("Template information")
            pprint(responseInstanceTemplateInfo["properties"])

        machine_type = responseInstanceTemplateInfo["properties"]["machineType"]
        is_preemtible = responseInstanceTemplateInfo["properties"]["scheduling"][
            "preemptible"
        ]
        if self.debug > 0:
            print("Machine Type: " + machine_type)
            print("Is preemtible: " + str(is_preemtible))
        request = self.service.machineTypes().get(
            project=self.project, zone=self.zone, machineType=machine_type
        )
        response = request.execute()
        guest_cpus = response["guestCpus"]
        if self.debug > 1:
            print("Machine information")
            pprint(responseInstanceTemplateInfo["properties"])
        if self.debug > 0:
            print("Guest CPUs: " + str(guest_cpus))

        instanceTemplateInfo = {
            "machine_type": machine_type,
            "is_preemtible": is_preemtible,
            "guest_cpus": guest_cpus,
        }
        return instanceTemplateInfo

    def scale(self):
        # diagnosis
        if self.debug > 1:
            print("Launching autoscaler.py with the following arguments:")
            print("project_id: " + self.project)
            print("zone: " + self.zone)
            print("region: " + self.region)
            print(f"multizone: {self.multizone}")
            print("group_manager: " + self.instance_group_manager)
            print("computeinstancelimit: " + str(self.compute_instance_limit))
            print("debuglevel: " + str(self.debug))

        if self.multizone:
            self.zoneargs = {"region": self.region}
        else:
            self.zoneargs = {"zone": self.zone}

        # Count jobs that are running or idle ("potentially runnable"); do not
        # count held jobs or jobs in transitioning states
        # https://htcondor.readthedocs.io/en/latest/classad-attributes/job-classad-attributes.html#JobStatus
        schedd = htcondor.Schedd()
        job_attributes = [ "RequestCpus", "RequestMemory", "RequestGpus" ]
        running_job_ads = schedd.query(constraint="JobStatus==2", projection=job_attributes)
        idle_job_ads = schedd.query(constraint="JobStatus==1", projection=job_attributes)

        total_idle_request_cpus = sum(j["RequestCpus"] for j in idle_job_ads)
        total_running_request_cpus = sum(j["RequestCpus"] for j in running_job_ads)
        queue = total_idle_request_cpus + total_running_request_cpus

        print(f"Running job RequestCpus: {total_running_request_cpus}")
        print(f"Idle job RequestCpus: {total_idle_request_cpus}")

        instanceTemplateInfo = self.getInstanceTemplateInfo()
        if self.debug > 1:
            print("Information about the compute instance template")
            pprint(instanceTemplateInfo)

        self.cores_per_node = instanceTemplateInfo["guest_cpus"]
        print("Number of CPU per compute node: " + str(self.cores_per_node))

        # Calculate number instances to satisfy current job queue CPU requests
        if queue > 0:
            self.size = int(math.ceil(float(queue) / float(self.cores_per_node)))
            if self.debug > 0:
                print(
                    "Calculating size of MIG: ⌈"
                    + str(queue)
                    + "/"
                    + str(self.cores_per_node)
                    + "⌉ = "
                    + str(self.size)
                )
        else:
            self.size = 0

        # If compute instance limit is specified, can not start more instances then specified in the limit
        if self.compute_instance_limit > 0 and self.size > self.compute_instance_limit:
            self.size = self.compute_instance_limit
            print(
                "MIG target size will be limited by " + str(self.compute_instance_limit)
            )

        print("New MIG target size: " + str(self.size))

        # Get current number of instances in the MIG
        requestGroupInfo = self.instanceGroupManagers.get(
            project=self.project,
            **self.zoneargs,
            instanceGroupManager=self.instance_group_manager,
        )
        responseGroupInfo = requestGroupInfo.execute()
        currentTarget = int(responseGroupInfo["targetSize"])
        print("Current MIG target size: " + str(currentTarget))

        if self.debug > 1:
            print("MIG Information:")
            print(responseGroupInfo)

        if self.size == 0 and currentTarget == 0:
            print(
                "No jobs in the queue and no compute instances running. Nothing to do"
            )
            exit()

        if self.size == currentTarget:
            print(
                "Running correct number of compute nodes to handle number of jobs in the queue"
            )
            exit()

        if self.size < currentTarget:
            print("Scaling down. Looking for nodes that can be shut down")

            # Find VMs that are unused (no dynamic slots created from
            # partitionable slots) and have been booted for at least 150 seconds
            max_daemon_start_time = int(datetime.datetime.now().timestamp()-150)
            filter_idle_vms = classad.ExprTree("PartitionableSlot && NumDynamicSlots==0")
            filter_uptime = classad.ExprTree(f"DaemonStartTime<{max_daemon_start_time}")

            coll = htcondor.Collector()
            idle_node_ads = coll.query(htcondor.AdTypes.Startd,
                constraint=filter_idle_vms.and_(filter_uptime),
                projection=["Machine", "CloudZone"])

            if self.debug > 1:
                print("Compute node busy status:")
                for node_ad in idle_node_ads:
                    print(node_ad["Machine"])

            # Shut down nodes that are not busy
            for node_ad in idle_node_ads:
                try:
                    node = node_ad["Machine"].split(".")[0]
                    zone = node_ad["CloudZone"]
                except KeyError:
                    print(f"Skipping ad: {node_ad}")
                    continue

                print("Will shut down: " + node + " ...")
                respDel = self.deleteFromMig(node, zone)
                if self.debug > 1:
                    print("Shut down request for compute node " + node)
                    pprint(respDel)

            if self.debug > 1:
                print("Scaling down complete")

        if self.size > currentTarget:
            print(
                "Scaling up. Need to increase number of instances to " + str(self.size)
            )
            # Request to resize
            request = self.instanceGroupManagers.resize(
                project=self.project,
                **self.zoneargs,
                instanceGroupManager=self.instance_group_manager,
                size=self.size,
            )
            response = request.execute()
            if self.debug > 1:
                print("Requesting to increase MIG size")
                pprint(response)
                print("Scaling up complete")


def main():

    scaler = AutoScaler(args.mz)

    # Project ID
    scaler.project = args.p  # Ex:'slurm-var-demo'

    # Name of the zone where the managed instance group is located
    scaler.zone = args.z  # Ex: 'us-central1-f'

    # Name of the region where the managed instance group is located
    scaler.region = args.r  # Ex: 'us-central1'

    # The name of the managed instance group.
    scaler.instance_group_manager = args.g  # Ex: 'condor-compute-igm'

    # Default number of cores per intance, will be replaced with actual value
    scaler.cores_per_node = 4

    # Default number of running instances that the managed instance group should maintain at any given time. This number will go up and down based on the load (number of jobs in the queue)
    scaler.size = 0

    # Dry run: : 0, run scaling; 1, only provide info.
    scaler.dryrun = args.d > 0

    # Debug level: 1-print debug information, 2 - print detail debug information
    scaler.debug = 0
    if args.v:
        scaler.debug = args.v

    # Limit for the maximum number of compute instance. If zero (default setting), no limit will be enforced by the  script
    scaler.compute_instance_limit = 0
    if args.c:
        scaler.compute_instance_limit = abs(args.c)

    scaler.scale()


if __name__ == "__main__":
    main()
