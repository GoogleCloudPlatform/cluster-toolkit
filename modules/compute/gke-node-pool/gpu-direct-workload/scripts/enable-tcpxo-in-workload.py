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

import yaml
import argparse
import os

def main():
    parser = argparse.ArgumentParser(description="TCPXO Job Manifest Generator")
    parser.add_argument("-f", "--file", required=True, help="Path to your job template YAML file")
    parser.add_argument("-r", "--rxdm", required=True, help="RxDM version")

    args = parser.parse_args()

    # Get the YAML file from the user
    if not args.file:
        args.file = input("Please provide the path to your job template YAML file: ")

    # Get component versions from user
    if not args.rxdm:
        args.rxdm = input("Enter the RxDM version: ")

    # Load and modify the YAML
    with open(args.file, "r") as file:
        job_manifest = yaml.load(file, Loader=yaml.BaseLoader)

    # Update annotations
    add_annotations(job_manifest)

    # Update volumes
    add_volumes(job_manifest)

    # Update tolerations
    add_tolerations(job_manifest)

    # Add tcpxo-daemon container
    add_tcpxo_daemon_container(job_manifest, args.rxdm)

    # Update environment variables and volumeMounts for GPU containers
    update_gpu_containers(job_manifest)

    # Generate the new YAML file
    updated_job = str(yaml.dump(job_manifest, default_flow_style=False, width=1000, default_style="|", sort_keys=False)).replace("|-", "")

    new_file_name = args.file.replace(".yaml", "-tcpxo.yaml")
    with open(new_file_name, "w", encoding="utf-8") as file:
        file.write(updated_job)

    # Step 7: Provide instructions to the user
    print("\nA new manifest has been generated and updated to have TCPXO enabled based on the provided workload")
    print("It can be found in {path}".format(path=os.path.abspath(new_file_name)))
    print("You can use the following commands to submit the sample job:")
    print("  kubectl create -f {path}".format(path=os.path.abspath(new_file_name)))

def add_annotations(job_manifest):
    annotations = {
        'devices.gke.io/container.tcpxo-daemon':"""|+
- path: /dev/nvidia0
- path: /dev/nvidia1
- path: /dev/nvidia2
- path: /dev/nvidia3
- path: /dev/nvidia4
- path: /dev/nvidia5
- path: /dev/nvidia6
- path: /dev/nvidia7
- path: /dev/nvidiactl
- path: /dev/nvidia-uvm
- path: /dev/dmabuf_import_helper""",
        "networking.gke.io/default-interface": "eth0",
        "networking.gke.io/interfaces": """|
[
    {"interfaceName":"eth0","network":"default"},
    {"interfaceName":"eth1","network":"vpc1"},
    {"interfaceName":"eth2","network":"vpc2"},
    {"interfaceName":"eth3","network":"vpc3"},
    {"interfaceName":"eth4","network":"vpc4"},
    {"interfaceName":"eth5","network":"vpc5"},
    {"interfaceName":"eth6","network":"vpc6"},
    {"interfaceName":"eth7","network":"vpc7"},
    {"interfaceName":"eth8","network":"vpc8"}
]""",
    }

    # Create path if it doesn't exist
    job_manifest.setdefault("spec", {}).setdefault("template", {}).setdefault("metadata", {})

    # Add/update annotations
    pod_template_spec = job_manifest["spec"]["template"]["metadata"]
    if "annotations" in pod_template_spec:
        pod_template_spec["annotations"].update(annotations)
    else:
        pod_template_spec["annotations"] = annotations

def add_tolerations(job_manifest):
    tolerations = [
        {"key": "user-workload", "operator": "Equal", "value": """\"true\"""", "effect": "NoSchedule"},
    ]

    # Create path if it doesn't exist
    job_manifest.setdefault("spec", {}).setdefault("template", {}).setdefault("spec", {})

    # Add tolerations
    pod_spec = job_manifest["spec"]["template"]["spec"]
    if "tolerations" in pod_spec:
        pod_spec["tolerations"].extend(tolerations)
    else:
        pod_spec["tolerations"] = tolerations

def add_volumes(job_manifest):
    volumes = [
        {"name": "nvidia-install-dir-host", "hostPath": {"path": "/home/kubernetes/bin/nvidia"}},
        {"name": "sys", "hostPath": {"path": "/sys"}},
        {"name": "proc-sys", "hostPath": {"path": "/proc/sys"}},
        {"name": "aperture-devices", "hostPath": {"path": "/dev/aperture_devices"}},
    ]

    # Create path if it doesn't exist
    job_manifest.setdefault("spec", {}).setdefault("template", {}).setdefault("spec", {})

    # Add volumes
    pod_spec = job_manifest["spec"]["template"]["spec"]
    if "volumes" in pod_spec:
        pod_spec["volumes"].extend(volumes)
    else:
        pod_spec["volumes"] = volumes


def add_tcpxo_daemon_container(job_template, rxdm_version):
    tcpxo_daemon_container = {
        "name": "tcpxo-daemon",
        "image": f"us-docker.pkg.dev/gce-ai-infra/gpudirect-tcpxo/tcpgpudmarxd-dev:{rxdm_version}",   # Use provided RxDM version
        "imagePullPolicy": "Always",
        "command": ["/bin/sh", "-c"],
        "args": [
            """|
                set -ex
                chmod 755 /fts/entrypoint_rxdm_container.sh
                /fts/entrypoint_rxdm_container.sh --num_hops=2 --num_nics=8 --uid= --alsologtostderr"""
        ],
        "securityContext": {
            "capabilities": {"add": ["NET_ADMIN", "NET_BIND_SERVICE"]}
        },
        "volumeMounts": [
            {"name": "nvidia-install-dir-host", "mountPath": "/usr/local/nvidia"},
            {"name": "sys", "mountPath": "/hostsysfs"},
            {"name": "proc-sys", "mountPath": "/hostprocsysfs"},
        ],
        "env": [{"name": "LD_LIBRARY_PATH", "value": "/usr/local/nvidia/lib64"}],
    }

    # Create path if it doesn't exist
    job_template.setdefault("spec", {}).setdefault("template", {}).setdefault("spec", {})

    # Add container
    pod_spec = job_template["spec"]["template"]["spec"]
    pod_spec.setdefault("containers", []).insert(0, tcpxo_daemon_container)

def update_gpu_containers(job_manifest):
    env_vars = [
        {"name": "LD_LIBRARY_PATH", "value": "/usr/local/nvidia/lib64"},
        {"name": "NCCL_FASTRAK_LLCM_DEVICE_DIRECTORY", "value": "/dev/aperture_devices"},
    ]
    volume_mounts = [{"name": "aperture-devices", "mountPath": "/dev/aperture_devices"}]

    pod_spec = job_manifest.get("spec", {}).get("template", {}).get("spec", {})
    for container in pod_spec.get("containers", []):
        # Create path if it doesn't exist
        container.setdefault("env", [])
        container.setdefault("volumeMounts", [])
        if int(container.get("resources", {}).get("limits", {}).get("nvidia.com/gpu", 0)) > 0:
            container["env"].extend(env_vars)
            container["volumeMounts"].extend(volume_mounts)

if __name__ == "__main__":
    main()
