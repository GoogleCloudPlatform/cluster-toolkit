#!/usr/bin/python3
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
from collections import OrderedDict
from datetime import datetime
from pprint import pprint
from googleapiclient import discovery
from oauth2client.client import GoogleCredentials

import argparse
import os
import math
import time
import htcondor
import classad

parser = argparse.ArgumentParser(formatter_class=argparse.ArgumentDefaultsHelpFormatter)
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
    "--i",
    default=0,
    help="Minimum number of idle compute instances",
    type=int
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

    # Remove specified instances from MIG and decrease MIG size
    def deleteFromMig(self, node_self_links):
        requestDelInstance = self.instanceGroupManagers.deleteInstances(
            project=self.project,
            **self.zoneargs,
            instanceGroupManager=self.instance_group_manager,
            body={ "instances": node_self_links },
        )

        # execute if not a dry-run
        if not self.dryrun:
            response = requestDelInstance.execute()
            if self.debug > 0:
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
        is_spot = responseInstanceTemplateInfo["properties"]["scheduling"][
            "preemptible"
        ]
        if self.debug > 0:
            print("Machine Type: " + machine_type)
            print("Is spot: " + str(is_spot))
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
            "is_spot": is_spot,
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

        # Each HTCondor scheduler (SchedD), maintains a list of jobs under its
        # stewardship. A full list of Job ClassAd attributes can be found at
        # https://htcondor.readthedocs.io/en/latest/classad-attributes/job-classad-attributes.html
        schedd = htcondor.Schedd()
        # encourage the job queue to start a new negotiation cycle; there are
        # internal unconfigurable rate limits so not guaranteed; this is not
        # strictly required for success, but may reduce latency of autoscaling
        schedd.reschedule()
        REQUEST_CPUS_ATTRIBUTE = "RequestCpus"
        REQUEST_GPUS_ATTRIBUTE = "RequestGpus"
        REQUEST_MEMORY_ATTRIBUTE = "RequestMemory"
        job_attributes = [
            REQUEST_CPUS_ATTRIBUTE,
            REQUEST_GPUS_ATTRIBUTE,
            REQUEST_MEMORY_ATTRIBUTE,
        ]

        instanceTemplateInfo = self.getInstanceTemplateInfo()
        self.is_spot = instanceTemplateInfo["is_spot"]
        self.cores_per_node = instanceTemplateInfo["guest_cpus"]
        print(f"MIG is configured for Spot pricing: {self.is_spot}")
        print("Number of CPU per compute node: " + str(self.cores_per_node))

        # this query will constrain the search for jobs to those that either
        # require spot VMs or do not require Spot VMs based on whether the
        # VM instance template is configured for Spot pricing
        spot_query = classad.ExprTree(f"RequireId == \"{self.instance_group_manager}\"")

        # For purpose of scaling a Managed Instance Group, count only jobs that
        # are idle and likely participated in a negotiation cycle (there does
        # not appear to be a single classad attribute for this).
        # https://htcondor.readthedocs.io/en/latest/classad-attributes/job-classad-attributes.html#JobStatus
        LAST_CYCLE_ATTRIBUTE = "LastNegotiationCycleTime0"
        coll = htcondor.Collector()
        negotiator_ad = coll.query(htcondor.AdTypes.Negotiator, projection=[LAST_CYCLE_ATTRIBUTE])
        if len(negotiator_ad) != 1:
            print(f"There should be exactly 1 negotiator in the pool. There is {len(negotiator_ad)}")
            exit()
        last_negotiation_cycle_time = negotiator_ad[0].get(LAST_CYCLE_ATTRIBUTE)
        if not last_negotiation_cycle_time:
            print(f"The negotiator has not yet started a match cycle. Exiting auto-scaling.")
            exit()

        print(f"Last negotiation cycle occurred at: {datetime.fromtimestamp(last_negotiation_cycle_time)}")
        idle_job_query = classad.ExprTree(f"JobStatus == 1 && QDate < {last_negotiation_cycle_time}")
        idle_job_ads = schedd.query(constraint=idle_job_query.and_(spot_query),
                                    projection=job_attributes)

        total_idle_request_cpus = sum(j[REQUEST_CPUS_ATTRIBUTE] for j in idle_job_ads)
        print(f"Total CPUs requested by idle jobs: {total_idle_request_cpus}")

        if self.debug > 1:
            print("Information about the compute instance template")
            pprint(instanceTemplateInfo)

        # Calculate the minimum number of instances that, for fully packed
        # execute points, could satisfy current job queue
        min_hosts_for_idle_jobs = math.ceil(total_idle_request_cpus / self.cores_per_node)
        if self.debug > 0:
            print(f"Minimum hosts needed: {total_idle_request_cpus} / {self.cores_per_node} = {min_hosts_for_idle_jobs}")

        # Get current number of instances in the MIG
        requestGroupInfo = self.instanceGroupManagers.get(
            project=self.project,
            **self.zoneargs,
            instanceGroupManager=self.instance_group_manager,
        )
        responseGroupInfo = requestGroupInfo.execute()
        current_target = responseGroupInfo["targetSize"]
        print(f"Current MIG target size: {current_target}")

        # Find instances that are being modified by the MIG (currentAction is
        # any value other than "NONE"). A common reason an instance is modified
        # is it because it has failed a health check.
        reqModifyingInstances = self.instanceGroupManagers.listManagedInstances(
            project=self.project,
            **self.zoneargs,
            instanceGroupManager=self.instance_group_manager,
            filter="currentAction != \"NONE\"",
            orderBy="creationTimestamp desc"
        )
        respModifyingInstances = reqModifyingInstances.execute()

        # Find VMs that are idle (no dynamic slots created from partitionable
        # slots) in the MIG handled by this autoscaler
        filter_idle_vms = classad.ExprTree(f"PartitionableSlot && NumDynamicSlots==0")
        filter_claimed_vms = classad.ExprTree(f"PartitionableSlot && NumDynamicSlots>0")
        filter_mig = classad.ExprTree(f"regexp(\".*/{self.instance_group_manager}$\", CloudCreatedBy)")
        # A full list of Machine (StartD) ClassAd attributes can be found at
        # https://htcondor.readthedocs.io/en/latest/classad-attributes/machine-classad-attributes.html
        idle_node_ads = coll.query(htcondor.AdTypes.Startd,
            constraint=filter_idle_vms.and_(filter_mig),
            projection=["Machine", "CloudZone"])

        NODENAME_ATTRIBUTE = "Machine"
        claimed_node_ads = coll.query(htcondor.AdTypes.Startd,
            constraint=filter_claimed_vms.and_(filter_mig),
            projection=[NODENAME_ATTRIBUTE])
        claimed_nodes = [ ad[NODENAME_ATTRIBUTE].split(".")[0] for ad in claimed_node_ads]

        # treat OrderedDict as a set by ignoring key values; this set will
        # contain VMs we would consider deleting, in inverse order of
        # their readiness to join pool (creating, unhealthy, healthy+idle)
        idle_nodes = OrderedDict()
        try:
            modifyingInstances = respModifyingInstances["managedInstances"]
        except KeyError:
            modifyingInstances = []

        print(f"There are {len(modifyingInstances)} VMs being modified by the managed instance group")

        # there is potential for nodes in MIG health check "VERIFYING" state
        # to have already joined the pool and be running jobs
        for instance in modifyingInstances:
            self_link = instance["instance"]
            node_name = self_link.rsplit("/", 1)[-1]
            if node_name not in claimed_nodes:
                idle_nodes[self_link] = "modifying"

        for ad in idle_node_ads:
            node = ad["Machine"].split(".")[0]
            zone = ad["CloudZone"]
            self_link = "https://www.googleapis.com/compute/v1/projects/" + \
                self.project + "/zones/" + zone + "/instances/" + node
            # there is potential for nodes in MIG health check "VERIFYING" state
            # to have already joined the pool and be idle; delete them last
            if self_link in idle_nodes:
                idle_nodes.move_to_end(self_link)
            idle_nodes[self_link] = "idle"
        n_idle = len(idle_nodes)

        print(f"There are {n_idle} VMs being modified or idle in the pool")
        if self.debug > 1:
            print("Listing idle nodes:")
            pprint(idle_nodes)

        # always keep size tending toward the minimum idle VMs requested
        new_target = current_target + self.compute_instance_min_idle - n_idle + min_hosts_for_idle_jobs
        if new_target > self.compute_instance_limit:
            self.size = self.compute_instance_limit
            print(f"MIG target size will be limited by {self.compute_instance_limit}")
        else:
            self.size = new_target

        print(f"New MIG target size: {self.size}")

        if self.debug > 1:
            print("MIG Information:")
            print(responseGroupInfo)

        if self.size == current_target:
            if current_target == 0:
                print("Queue is empty")
            print("Running correct number of VMs to handle queue")
            exit()

        if self.size < current_target:
            print("Scaling down. Looking for nodes that can be shut down")

            if self.debug > 1:
                print("Compute node busy status:")
                for node in idle_nodes:
                    print(node)

            # Shut down idle nodes up to our calculated limit
            nodes_to_delete = list(idle_nodes.keys())[0:current_target-self.size]
            for node in nodes_to_delete:
                print(f"Attempting to delete: {node.rsplit('/',1)[-1]}")
            respDel = self.deleteFromMig(nodes_to_delete)

            if self.debug > 1:
                print("Scaling down complete")

        if self.size > current_target:
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

    # Default number of cores per instance, will be replaced with actual value
    scaler.cores_per_node = 4

    # Default number of running instances that the managed instance group should maintain at any given time. This number will go up and down based on the load (number of jobs in the queue)
    scaler.size = 0

    scaler.compute_instance_min_idle = args.i

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
