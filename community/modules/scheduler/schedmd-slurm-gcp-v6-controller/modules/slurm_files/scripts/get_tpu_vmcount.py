#!/usr/bin/env python3

# Copyright 2024 Google Inc. All rights reserved.
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

import argparse
import util
import tpu


def get_vmcount_of_tpu_part(part):
    res = 0
    lkp = util.lookup()
    for ns in lkp.cfg.partitions[part].partition_nodeset_tpu:
        tpu_obj = tpu.TPU.make(ns, lkp)
        if res == 0:
            res = tpu_obj.vmcount
        else:
            if res != tpu_obj.vmcount:
                # this should not happen, that in the same partition there are different vmcount nodesets
                return -1
    return res


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--partitions",
        "-p",
        help="The partition(s) to retrieve the TPU vmcount value for.",
    )
    args = parser.parse_args()
    if not args.partitions:
        exit(0)

    # useful exit code
    # partition does not exists in config.yaml, thus do not exist in slurm
    PART_INVALID = -1
    # in the same partition there are nodesets with different vmcounts
    DIFF_VMCOUNTS_SAME_PART = -2
    # partition is a list of partitions in which at least two of them have different vmcount
    DIFF_PART_DIFFERENT_VMCOUNTS = -3
    vmcounts = []
    # valid equals to 0 means that we are ok, otherwise it will be set to one of the previously defined exit codes
    valid = 0
    for part in args.partitions.split(","):
        if part not in util.lookup().cfg.partitions:
            valid = PART_INVALID
            break
        else:
            if util.lookup().partition_is_tpu(part):
                vmcount = get_vmcount_of_tpu_part(part)
                if vmcount == -1:
                    valid = DIFF_VMCOUNTS_SAME_PART
                    break
                vmcounts.append(vmcount)
            else:
                vmcounts.append(0)
    # this means that there are different vmcounts for these partitions
    if valid == 0 and len(set(vmcounts)) != 1:
        valid = DIFF_PART_DIFFERENT_VMCOUNTS
    if valid != 0:
        print(f"VMCOUNT:{valid}")
    else:
        print(f"VMCOUNT:{vmcounts[0]}")
