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

import glob
import json
import hashlib

# OFE-deployment test is configured to only run as a PR trigger and does
# not run on a nightly basis. Refer tools/cloud-build/provision/pr-ofe-test.tf
# for the configuration.
TO_SKIP = frozenset(["ofe-deployment"])

# Seed for deterministic order of tests, change to other value to shuffle tests
ORDER_SEED = b"Hakuna Matata"

def list_builds() -> list[str]:
    builds = [b[:-5] for b in glob.glob("*.yaml", root_dir="../daily-tests/builds/")]
    assert builds, "No builds have been found"
    return list(set(builds) - TO_SKIP)

HASH = lambda s: int(hashlib.md5(s.encode() + ORDER_SEED).hexdigest(), 16)

def schedule_evenly(builds: list[str], start: int, end: int) -> dict[str, str]:
    """
    Schedule builds evenly between start and end time.
    """
    # use hash instead of names to avoid clustering of similar tests
    order = sorted(builds, key=HASH)
    interval = (end - start) / max(1, len(builds) - 1)
    return {b: int(start + i * interval) for i, b in enumerate(order)}

# DO_NOT_SUBMIT: please review the proposed change
def schedule_consistently(builds: list[str], start: int, end: int) -> dict[str, str]:
    duration = max(end - start, 1)
    coord = lambda b: start + (HASH(b) % duration)
    return {b: coord(b) for b in sorted(builds, key=coord)}

def crontab(schedule: dict[str, int]) -> dict[str, str]:
    return { # test: "{minutes} {hours} * * MON-FRI"
        k: f"{t % 60} {t // 60} * * MON-FRI" for k, t in schedule.items()}

if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('start_time', type=int,
                        help='minutes since midnight to run the first test, e.g. 30 = 00:30')
    parser.add_argument('end_time', type=int,
                        help='minutes since midnight to run the last test, e.g. 300 = 05:00')
    args = parser.parse_args()

    assert args.start_time < args.end_time
    builds = list_builds()
    schedule = schedule_evenly(builds, args.start_time, args.end_time)
    #schedule = schedule_consistently(builds, args.start_time, args.end_time)
    print(json.dumps(crontab(schedule)))
