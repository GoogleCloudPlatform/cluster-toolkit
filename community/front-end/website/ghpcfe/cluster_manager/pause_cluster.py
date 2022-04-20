#!/usr/bin/env python3
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

"""Pause cluster logic (not yet implemented)"""


from .utils import load_cluster_info, run_terraform


def pause_cluster(args):
    """
    Pause or un-pause a cluster.

        Parameters:
            args - an object with the following members:
                'cluster_id' - id # of the compute cluster
                'accessKey' - key to update database  [not really used ?]
    """
    load_cluster_info(args)

    if args.cloud == "google":
        raise NotImplementedError("Pausing on GCP not yet implemented")
        # pause_cluster_gcp(args)
    else:
        raise NotImplementedError(
            f"Pausing on {args.cloud} not yet implemented"
        )

    run_terraform(args.cluster_dir / "terraform" / args.cloud, "refresh")
