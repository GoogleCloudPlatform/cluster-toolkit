## Slurm Troubleshooting

### Network is unreachable (Slurm V5)

Slurm requires access to google APIs to function. This can be achieved through one of the following methods:

1. Create a [Cloud NAT](https://cloud.google.com/nat) (preferred).
2. Setting `disable_controller_public_ips: false` &
   `disable_login_public_ips: false` on the controller and login nodes
   respectively.
3. Enable
   [private access to Google APIs](https://cloud.google.com/vpc/docs/private-access-options).

By default the Toolkit VPC module will create an associated Cloud NAT so this is
typically seen when working with the pre-existing-vpc module. If no access
exists you will see the following errors:

When you ssh into the login node or controller you will see the following
message:

```text
*** Slurm setup failed! Please view log: /slurm/scripts/setup.log ***
```

> **_NOTE:_**: Many different potential issues could be indicated by the above
> message, so be sure to verify issue in logs.

To confirm the issue, ssh onto the controller and call `sudo cat /slurm/scripts/setup.log`. Look for
the following logs:

```text
google_metadata_script_runner: startup-script: ERROR: [Errno 101] Network is unreachable
google_metadata_script_runner: startup-script: OSError: [Errno 101] Network is unreachable
google_metadata_script_runner: startup-script: ERROR: Aborting setup...
google_metadata_script_runner: startup-script exit status 0
google_metadata_script_runner: Finished running startup scripts.
```

You may also notice mount failure logs on the login node:

```text
INFO: Waiting for '/usr/local/etc/slurm' to be mounted...
INFO: Waiting for '/home' to be mounted...
INFO: Waiting for '/opt/apps' to be mounted...
INFO: Waiting for '/etc/munge' to be mounted...
ERROR: mount of path '/usr/local/etc/slurm' failed: <class 'subprocess.CalledProcessError'>: Command '['mount', '/usr/local/etc/slurm']' returned non-zero exit status 32.
ERROR: mount of path '/opt/apps' failed: <class 'subprocess.CalledProcessError'>: Command '['mount', '/opt/apps']' returned non-zero exit status 32.
ERROR: mount of path '/home' failed: <class 'subprocess.CalledProcessError'>: Command '['mount', '/home']' returned non-zero exit status 32.
ERROR: mount of path '/etc/munge' failed: <class 'subprocess.CalledProcessError'>: Command '['mount', '/etc/munge']' returned non-zero exit status 32.
```

> **_NOTE:_**: The above logs only indicate that something went wrong with the
> startup of the controller. Check logs on the controller to be sure it is a
> network issue.

### Failure to Create Auto Scale Nodes (Slurm)

If your deployment succeeds but your jobs fail with the following error:

```shell
$ srun -N 6 -p compute hostname
srun: PrologSlurmctld failed, job killed
srun: Force Terminated job 2
srun: error: Job allocation 2 has been revoked
```

Possible causes could be [insufficient quota](#insufficient-quota),
[placement groups](#placement-groups-slurm), or
[insufficient permissions](#insufficient-service-account-permissions)
for the service account attached to the controller. Also see the
[Slurm user guide](https://docs.google.com/document/u/1/d/e/2PACX-1vS0I0IcgVvby98Rdo91nUjd7E9u83oIMCM4arne-9_IdBg6BdV1lBpUcSje_PyHcbAaErC1rY7p4u1g/pub).

#### Insufficient Quota

It may be that you have sufficient quota to deploy your cluster but insufficient
quota to bring up the compute nodes.

You can confirm this by SSHing into the `controller` VM and checking the
`resume.log` file:

```shell
$ cat /var/log/slurm/resume.log
...
resume.py ERROR: ... "Quota 'C2_CPUS' exceeded. Limit: 300.0 in region europe-west4.". Details: "[{'message': "Quota 'C2_CPUS' exceeded. Limit: 300.0 in region europe-west4.", 'domain': 'usageLimits', 'reason': 'quotaExceeded'}]">
```

The solution here is to [request more of the specified quota](#gcp-quotas),
`C2 CPUs` in the example above. Alternatively, you could switch the partition's
[machine type][partition-machine-type], to one which has sufficient quota.

[partition-machine-type]: community/modules/compute/schedmd-slurm-gcp-v6-nodeset/README.md#input_machine_type

#### Placement Groups (Slurm)

By default, placement groups (also called affinity groups) are enabled on the
compute partition. This places VMs close to each other to achieve lower network
latency. If it is not possible to provide the requested number of VMs in the
same placement group, the job may fail to run.

Again, you can confirm this by SSHing into the `controller` VM and checking the
`resume.log` file:

```shell
$ cat /var/log/slurm/resume.log
...
resume.py ERROR: group operation failed: Requested minimum count of 6 VMs could not be created.
```

One way to resolve this is to set [enable_placement][partition-enable-placement]
to `false` on the partition in question.

[partition-enable-placement]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/compute/schedmd-slurm-gcp-v6-nodeset#input_enable_placement

#### VMs Get Stuck in Status Staging When Using Placement Groups With vm-instance

If VMs get stuck in `status: staging` when using the `vm-instance` module with
placement enabled, it may be because you need to allow terraform to make more
concurrent requests. See
[this note](modules/compute/vm-instance/README.md#placement) in the vm-instance
README.

#### Insufficient Service Account Permissions

By default, the Slurm controller, login and compute nodes use the
[Google Compute Engine Service Account (GCE SA)][def-compute-sa]. If this
service account or a custom SA used by the Slurm modules does not have
sufficient permissions, configuring the controller or running a job in Slurm may
fail.

If configuration of the Slurm controller fails, the error can be
seen by viewing the startup script on the controller:

```shell
sudo journalctl -u google-startup-scripts.service | less
```

An error similar to the following indicates missing permissions for the service
account:

```shell
Required 'compute.machineTypes.get' permission for ...
```

To solve this error, ensure your service account has the
`compute.instanceAdmin.v1` IAM role:

```shell
SA_ADDRESS=<SET SERVICE ACCOUNT ADDRESS HERE>

gcloud projects add-iam-policy-binding ${PROJECT_ID} \
    --member=serviceAccount:${SA_ADDRESS} --role=roles/compute.instanceAdmin.v1
```

If Slurm failed to run a job, view the resume log on the controller instance
with the following command:

```shell
sudo cat /var/log/slurm/resume.log
```

An error in `resume.log` similar to the following indicates a permissions issue
as well:

```shell
The user does not have access to service account 'PROJECT_NUMBER-compute@developer.gserviceaccount.com'.  User: ''.  Ask a project owner to grant you the iam.serviceAccountUser role on the service account": ['slurm-hpc-small-compute-0-0']
```

As indicated, the service account must have the compute.serviceAccountUser IAM
role. This can be set with the following command:

```shell
SA_ADDRESS=<SET SERVICE ACCOUNT ADDRESS HERE>

gcloud projects add-iam-policy-binding ${PROJECT_ID} \
    --member=serviceAccount:${SA_ADDRESS} --role=roles/iam.serviceAccountUser
```

If the GCE SA is being used and cannot be updated, a new service account can be
created and used with the correct permissions. Instructions for how to do this
can be found in the [Slurm on Google Cloud User Guide][slurm-on-gcp-ug],
specifically the section titled "Create Service Accounts".

After creating the service account, it can be set via the
`compute_node_service_account` and `controller_service_account` settings on the
[slurm-on-gcp controller module][slurm-on-gcp-con] and the
"login_service_account" setting on the
[slurm-on-gcp login module][slurm-on-gcp-login].

[def-compute-sa]: https://cloud.google.com/compute/docs/access/service-accounts#default_service_account
[slurm-on-gcp-ug]: https://goo.gle/slurm-gcp-user-guide
[slurm-on-gcp-con]: community/modules/scheduler/schedmd-slurm-gcp-v6-controller/README.md
[slurm-on-gcp-login]: community/modules/scheduler/schedmd-slurm-gcp-v6-login/README.md

### Timeout Error / Startup Script Failure (Slurm V5)

If you observe failure of startup scripts in version 5 of the Slurm module,
they may be due to a 300 second maximum timeout on scripts. All startup script
logging is found in `/slurm/scripts/setup.log` on every node in a Slurm cluster.
The error will appear similar to:

```text
2022-01-01 00:00:00,000 setup.py DEBUG: custom scripts to run: /slurm/custom_scripts/(login_r3qmskc0.d/ghpc_startup.sh)
2022-01-01 00:00:00,000 setup.py INFO: running script ghpc_startup.sh
2022-01-01 00:00:00,000 util DEBUG: run: /slurm/custom_scripts/login_r3qmskc0.d/ghpc_startup.sh
2022-01-01 00:00:00,000 setup.py ERROR: TimeoutExpired:
    command=/slurm/custom_scripts/login_r3qmskc0.d/ghpc_startup.sh
    timeout=300
    stdout:
```

We anticipate that this limit will be configured in future releases of the Slurm
module, however we recommend that you use a dedicated build VM where possible
to execute scripts of significant duration. This pattern is demonstrated in the
[AMD-optimized Slurm cluster example](../community/examples/AMD/).

### Slurm Controller Startup Fails with `exportfs` Error

Example error in `/slurm/scripts/setup.log` (on Slurm V5 controller):

```text
exportfs: /****** does not support NFS export
```

This can be caused when you are mounting a Filestore that has the same name for
`local_mount` and `filestore_share_name`.

For example:

```yaml
  - id: samesharefs  # fails to exportfs
    source: modules/file-system/filestore
    use: [network1]
    settings:
      filestore_share_name: same
      local_mount: /same
```

This is a known issue, the recommended workaround is to use different naming for
the `local_mount` and `filestore_share_name`.

### `local-exec provisioner error` During Terraform Apply

Using the `enable_reconfigure` setting with Slurm v5 modules uses `local-exec`
provisioners to perform additional cluster configuration. Some common issues
experienced when using this feature are missing local python requirements and
incorrectly configured gcloud cli. There is more information about these issues
and fixes on the
[`schedmd-slurm-gcp-v5-controller` documentation](../community/modules/scheduler/schedmd-slurm-gcp-v5-controller/README.md#live-cluster-reconfiguration-enable_reconfigure).
