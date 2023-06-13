#!/usr/bin/env python3
# Copyright 2023 Google LLC
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

import glob
import json

"""
List build files in `tools/cloud-build/daily-tests/builds/`
and creates a cron-schedule-string for each of them.
E.g.:
```
$ ./list_tests.py | jq
{
  "batch-mpi": "30 0 * * *",
  "chrome-remote-desktop": "48 0 * * *",
  "cloud-batch": "6 1 * * *",
  "hpc-high-io-v4": "24 1 * * *",
  "hpc-high-io-v5": "42 1 * * *",
...
```
"""
def list_builds(start_time=30, end_time=5*60):
    """
    start_time: int, minutes since midnight to run the first test, e.g. 30 = 00:30
    end_time: int, minutes since midnight to run the last test, e.g. 5*60 = 05:00
    """
    builds = glob.glob("*.yaml", root_dir="../daily-tests/builds/")
    assert builds, "No builds have been found"
    # Sort and strip ".yaml"
    builds = [b[:-5] for b in sorted(builds)]
    interval = (end_time - start_time) // max(1, len(builds) - 1)
    res = {}
    for b in builds:
        h, m = start_time // 60, start_time % 60
        res[b] = f"{m} {h} * * MON-FRI"
        start_time += interval
    return res


if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('start_time', type=int,
                        help='minutes since midnight to run the first test, e.g. 30 = 00:30')
    parser.add_argument('end_time', type=int,
                        help='minutes since midnight to run the last test, e.g. 300 = 05:00')
    args = parser.parse_args()

    print(json.dumps(list_builds(args.start_time, args.end_time)))
