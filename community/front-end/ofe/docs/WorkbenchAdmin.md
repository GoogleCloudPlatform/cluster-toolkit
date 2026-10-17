# Open Front End (OFE) - Workbench Administration

## Workbench Management

The Workbench feature provides a way to create and control VertexAI
Workbenches, which provide a single interactive development
environment using a Jupyter Notebook perfect for pre/post processing
of data. Workbenches can be located within the same VPC as other GCP
resources managed by the OFE.

### Workbench Configuration

The first stage of configuration, after selecting the desired cloud
credential, is to select the basic profile of the workbench
including:

- Workbench Name
- Subnet
- Cloud Zone
- Trusted User

![Workbench create process part 1](images/Workbench-Create-1.png)

The subnet will define which regions the workbench can be located in.
Workbenches are not available in all regions, see
[Workbench Documentation](https://cloud.google.com/vertex-ai/docs/general/locations#vertex-ai-workbench-locations)
for more detail on currently available regions. Once a region is
selected the cloud zone field will be populated with the available
zones.

### Trusted User

The trusted user field will govern which user has access to the
workbench. This is a 1:1 relationship as each workbench has a single
instance owner that is set by the trusted user value. The user is
selected from the OFE's users, and their email address is passed to
Terraform as `trusted_user` and `owner_id`. The workbench
is then configured to run the jupyter notebook as the users OSLogin
account (found from their Google social account ID). Access to the notebook is controlled by a proxy that
requires the user to be logged into their google account to gain
access.

Workbench instances have a limited number of configurations:

- Machine type
- Boot disk type: Standard Persistent Disk (`PD_STANDARD`, the default),
  Balanced Persistent Disk (`PD_BALANCED`) or SSD Persistent Disk (`PD_SSD`)
- Boot disk capacity in GB (default and minimum 120)
- Image type
- Attached cluster (optional)

![Workbench create process part 2](images/Workbench-Create-2.png)

### Attached cluster

The optional *attached cluster* field lists clusters that are currently running
(cloud state "m"). The workbench and the cluster must share a VPC, otherwise
the form is rejected with "Cluster and workbench must share a vpc". When a
cluster is attached, the cluster's filesystem mount points are copied to the
workbench, and the workbench can be used to submit jobs to that cluster.

### Workbench status

| Status | Meaning |
| ------ | ------- |
| `n` | Newly configured; only a database record exists |
| `c` | Being created |
| `i` | Being initialised |
| `r` | Ready |
| `s` | Stopped (can be restarted) |
| `t` | Terminating |
| `e` | Deployment has failed |
| `d` | Destroyed |

The `s` status is defined in the model; the *Start Workbench* action in the
list is offered for newly configured (`n`) workbenches only.

### Machine type & Workbench Presets

An administrator can configure any type of machine type that is
available. Users with the "Normal User" class will only be able to
create workbenches using the preset machine type configurations while
users with the "Viewer" class will not be able to create workbenches
for themselves. The Open Front End (OFE) comes with some
pre-configured workbench presets:
- Small - 1x core with 3840 Memory (n1-standard-1)
- Medium - 2x cores with 7680 Memory (n1-standard-2)
- Large - 4x cores with 15360 Memory (n1-standard-4)
- X-Large - 8x cores with 30720 Memory (n1-standard-8)

Each of these have been created under the category "Recommended".
Presets can be edited, deleted or new presets added via the admin
panel where you can set the machine type and the category under which
the user will see the preset.

![Workbench create process - Presets](images/Workbench-Create-Presets.png)

### Workbench Storage

The final setup of the workbench is to select any filesystems that
are required to be mounted on the workbench. On this page the
configuration fields will be disabled and no changes will be possible
to the workbench configuration.

![Workbench create process - Storage](images/Workbench-Create-Storage.png)

Within this configuration you can select from existing storage
exports, the order they are mounted, and the mouth path in the
filesystem. Storage will be mounted in the order according to the
mount order which will be important if you are mounting storage
within a sub-directory of another storage mount. Another important
configuration to be aware of is that filesystems will only be
mounted if the filestore or cluster is active and has an accurate
IP address or hostname in the frontends database.

### Workbench problems

#### Storage not mounted
If the expected filesystem storage has not been mounted or is not
available the most likely cause is that the database does not have a
hostname or IP address for the filestore or cluster targeted. An
admin can resolve this by accessing the instance by SSH-ing into the
GCP instance the runs the workbench and running
`mount $IPADDRESS:/$TARGETDIR $MOUNTPOINT`

#### Workbench stuck in "Creating" status
If a workbench is stuck in "Creating" status this can be resolved by
manually changing the status back to newly created in the admin
portal and then starting the creation process again. Logs for this
process can be seen at
`$OFE_HOME/workbenches/workbench_##/terraform/google/`
where `$OFE_HOME` is normally
`/opt/gcluster/cluster-toolkit/community/front-end/ofe` and ## will be the id
number of the workbench in question.
