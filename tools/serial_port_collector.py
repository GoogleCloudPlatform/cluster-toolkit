#!/bin/python3
# Copyright 2024 "Google LLC"
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

import time
from google.cloud import compute_v1
from argparse import ArgumentParser, RawTextHelpFormatter

"""This tool collects serial port output and prints it to the terminal until
the VM is deleted or it hits the timeout (300s).  It takes in, project, vm_name
and zone as arguments.  The script should only print each line once, using the
line number of the previous serial port retrieval as the starting point of the
next request.

usage: serial_port_collector.py [-h] -p PROJECT -v VM_NAME -z ZONE [-t TIMEOUT]
"""

def get_serial_port_output(host_name: str, project: str, zone: str,
                           start: int = 0) -> str:
    # Create a client
    client = compute_v1.InstancesClient()
    # Initialize request argument(s)
    request = compute_v1.GetSerialPortOutputInstanceRequest(
        instance=host_name,
        project=project,
        zone=zone,
        start=start,
    )
    # Make the request
    res = client.get_serial_port_output(request=request)
    return res.contents, res.next_

if __name__ == "__main__":
    parser = ArgumentParser(prog='serial_port_collector.py',
                                     formatter_class=RawTextHelpFormatter)
    parser.add_argument("-p", "--project", required=True, type=str,
                        help="Project where the vm is located")
    parser.add_argument("-v", "--vm_name", required=True, type=str,
                        help="VM name to collect serial port output from")
    parser.add_argument("-z", "--zone", required=True, type=str,
                    help="The zone the vm is located in")
    parser.add_argument("-t", "--timeout", type=int, default = 0,
                        help="Timeout in seconds waiting for the next output "\
                             "(values <= 0 are no timeout)")

    args = parser.parse_args()
    to = args.timeout

    next=0
    sleep_timer = 2
    ts = time.time()
    while to <= 0 or time.time()-ts < to:
        out, next = get_serial_port_output(args.vm_name, args.project, 
                                           args.zone, next)
        if len(out) > 0:
            print(out)
            ts = time.time()
        time.sleep(sleep_timer)
