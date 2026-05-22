# GKE Upgrade Guide for Cluster Toolkit

This guide provides comprehensive instructions for Cluster Toolkit users on how to manage and upgrade Google Kubernetes Engine (GKE) versions. It covers strategies for selecting appropriate versions, deploying new clusters with specific version requirements, and performing manual upgrades on existing clusters across various release channels. This guide helps you keep your GKE clusters secure, up-to-date, and stable for HPC and AI/ML workloads.

## 1\. GKE Versioning and Release Channels

Google Kubernetes Engine (GKE) regularly releases new versions containing new features, performance improvements, and security patches. GKE offers Release Channels to manage the balance between feature availability and stability.

When planning an upgrade, you should review the official [GKE Release Notes](https://docs.cloud.google.com/kubernetes-engine/docs/release-notes) to understand the changes and identify the target version for your upgrade.

## 2\. Cluster Upgrade Strategy

Upgrade strategies differ depending on whether you are deploying a new cluster or updating an existing one. For new clusters, you can specify the target version or channel in the blueprint. For existing clusters, you perform a managed upgrade using `gcloud` commands.

### 2.1 Deploying a New Cluster with a Specific Version

When deploying a new cluster, you can configure your blueprint to use a specific GKE version or a release channel.

#### Step 1: Identify Target Version

Find the desired GKE version from the [GKE Release Notes](https://docs.cloud.google.com/kubernetes-engine/docs/release-notes). You should choose a version that is available in the channel you plan to use, or a version that meets your specific requirements (e.g., a specific patch for a bug fix). Note that versions available in the `RAPID` channel may not be available in `REGULAR` or `STABLE` yet.

#### Step 2: Update Blueprint

Modify your blueprint to specify the GKE version and release channel as shown below.

##### Specifying GKE Versions in Blueprint

In your Cluster Toolkit blueprint, you can control the GKE version via `version_prefix` var:  
Blueprint snippet:

```
# ...
deployment_groups:
- group: primary
  modules:
  - id: my-gke-cluster
    source: modules/scheduler/gke-cluster
    settings:
      # Specify the GKE version here
      # version_prefix: "1.30.1-gke.1156000"
      # ...
```

**Note** on `version_prefix`: By default, the `gke-cluster` module restricts upgrades to a specific minor version (like `1.31.`).

* To stay on the same minor version: Leave `version_prefix` as is.  
* To upgrade to a new minor version (or specific patch): You must update `version_prefix` to that new prefix or full version (e.g., `"1.32."` or `"1.32.2-gke.100"`).

##### Specifying Release Channels in Blueprint

GKE offers Release Channels (RAPID, REGULAR, STABLE) which can be configured in the `release_channel` variable.

* **RAPID Channel**: Receives new features and versions first.  
* **REGULAR Channel**: The default channel, balanced between stability and new features.  
* **STABLE Channel**: Receives versions after they have been thoroughly soaked in other channels.

Blueprint snippet:

```
# ...
deployment_groups:
- group: primary
  modules:
  - id: my-gke-cluster
    source: modules/scheduler/gke-cluster
    settings:
      release_channel: REGULAR
```

#### Step 3: Deploy the cluster

Run `gcluster deploy` to create the cluster. Sample command:

```shell
./gcluster deploy examples/<path-to-blueprint.yaml>
```

**Note**: For many machine types, deploy command would also have the deployment config file as well. Refer the respective READMEs of machine types in the toolkit for exact deployment commands.

### 2.2 Upgrading an Existing Cluster

To upgrade an existing cluster, you can perform a manual version upgrade to a specific target version or change the cluster's release channel to align with a different stability track. This section covers procedures for upgrading both the master control plane and individual node pools using either the **`gcloud` CLI** or the **Google Cloud Console (UI).**

### Upgrading via gcloud CLI

This operation updates the GKE version of the control plane and node pools of the cluster via the gcloud CLI.

#### Step 1: Identify Target Version

Find the target version from the [GKE Release Notes](https://docs.cloud.google.com/kubernetes-engine/docs/release-notes).

#### Step 2: Upgrade Master Control Plane

The GKE control plane must be upgraded before the nodes can be upgraded. Use the `gcloud` command with the `--master` flag.

```shell
gcloud container clusters upgrade CLUSTER_NAME \
    --master \
    --cluster-version=TARGET_GKE_VERSION \
    --region=REGION \
    --project=PROJECT_ID
```

#### Step 3: Upgrade Node Pools

Once the master is upgraded, upgrade each node pool (including system and compute pools) individually.

```shell
gcloud container clusters upgrade CLUSTER_NAME \
    --node-pool=NODE_POOL_NAME \
    --cluster-version=TARGET_GKE_VERSION \
    --region=REGION \
    --project=PROJECT_ID
```

#### Step 4: Verify Upgrade of Cluster and Node pool

Check that the nodes are running the target version:

* ##### Verify Cluster Version and Release Channel

**Command:**

```shell
gcloud container clusters describe CLUSTER_NAME \
    --region=REGION \
    --format="yaml(currentMasterVersion, releaseChannel)" \
    --project=PROJECT_ID
```

**Expected Output:**

```
currentMasterVersion: TARGET_GKE_VERSION
releaseChannel:
  channel: REGULAR
```

* ##### Verify Node Pool Version

**Command:**

```shell
gcloud container node-pools describe NODE_POOL_NAME \
    --cluster=CLUSTER_NAME \
    --region=REGION \
    --format="yaml(version)" \
    --project=PROJECT_ID
```

**Expected Output:**

```
version: TARGET_GKE_VERSION
```

#### Changing Release Channel via gcloud CLI

This command is typically used to move a cluster from the `RAPID` channel back to `REGULAR` or `STABLE` once the desired version becomes available in those channels, allowing you to resume automatic upgrades on a more conservative track.

To change the release channel of an existing cluster, use the `gcloud container clusters update` command:

```shell
gcloud container clusters update CLUSTER_NAME \
    --release-channel=CHANNEL \
    --region=REGION
```

*Replace `CHANNEL` with `rapid`, `regular`, `stable`, or `none`.*

### Upgrading via Google Cloud Console (UI)

#### Step 1: Upgrade Control Plane

1. Navigate to Clusters: In the Google Cloud Console, go to the **Kubernetes Engine \> Clusters** page.  
2. Select Cluster: Click the name of the cluster you want to upgrade to open the cluster details.

3. Edit Version: In the **Cluster basics** section, locate the **Version** field and click the edit icon or **Upgrade available** link.  
4. Upgrade Control Plane: Select the option to upgrade the cluster control plane.  
5. Save Changes: Select your target version and click **Save Changes** to start the upgrade.


#### Step 2: Upgrade Node Pools

1. After the control plane upgrade completes, click the **Nodes** tab on the cluster details page.
2. Click the name of the node pool you want to upgrade.

3. Click **Edit** at the top of the page.
4. Click **Change** next to **Node version**, select the target version, and click **Save**.


#### Changing Release Channel via Console (UI)

1. Under **Cluster basics** on the cluster details page, find the **Release channel** field and click **Edit**.
2. Select the new channel and click **Save Changes**.

## 3\. Cluster Toolkit Team Updates

Periodically, the Cluster Toolkit team will update the default versions defined in the modules to incorporate new stable GKE versions. To adopt these new defaults:

1. **Update Toolkit**: You should update your Cluster Toolkit installation by downloading the latest cluster toolkit binary.  
2. **Re-Deploy**: Regenerate your Terraform code from your blueprints using the updated toolkit and apply the changes. This ensures you pick up the new defaults without necessarily hardcoding versions in your blueprints.

```shell
./gcluster deploy -d PATH_TO_DEPLOYMENT_FILE PATH_TO_BLUEPRINT_FILE.yaml
```

## 4\. Important Considerations During Upgrades

When performing manual upgrades, following aspects need to be considered:

* **Disruption and Reboots**: Upgrading GKE clusters requires node replacement or restarts. Using `gcloud container clusters upgrade CLUSTER_NAME --node-pool=NODE_POOL_NAME` will trigger a rolling update of that node pool, replacing old nodes with new ones running the updated version. This process is disruptive as pods will be evicted and rescheduled.  
* **Pod Disruption Budgets (PDBs)**: GKE respects PDBs during upgrades. If a PDB is overly restrictive, it can block or slow down the upgrade process. Conversely, ensure PDBs are configured correctly to maintain application availability during the rolling update.  
* **Storage Persistence**: Temporary storage volumes (like `emptyDir`) are deleted when nodes are replaced. Persistent disks (PVs) are unaffected.  
* **Channel Changes**:  
  * No Immediate Disruption: Changing the release channel is a metadata operation on the control plane and does not cause nodes to restart or be replaced, provided the current cluster version is valid in the new channel.  
  * Version Compatibility: The cluster's current version must be supported in the target channel. You may need to upgrade the cluster first if it's on a version too old for the new channel.  
* **Monitoring Upgrade Progress**: Monitor the upgrade progress to ensure nodes successfully transition to the new version. You can monitor the status in the Google Cloud Console under the GKE section.
