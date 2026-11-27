# Export deployments to Cluster Director

Cluster Toolkit can export a deployed Compute Engine or Slurm cluster to
[Cluster Director](https://docs.cloud.google.com/cluster-director/docs) so you
can view and monitor it there. The export registers your existing resources with
Cluster Director as an imported cluster. It doesn't create, change, or replace
any of your infrastructure: Cluster Toolkit and Terraform remain the source of
truth for provisioning, scaling, updating, and destroying your resources.

> [!NOTE]
> Exporting to Cluster Director is in preview and is **off by default**. Your
> Google Cloud project might need to be enabled for importing existing clusters
> into Cluster Director before you can use it. Contact your Google Cloud account
> team if your project doesn't have access.

## What gets exported

After `gcluster deploy` finishes applying Terraform, `gcluster` reads the
Terraform state of the deployment and exports:

- the Compute Engine instances of the deployment, which it selects by their
  existing `ghpc_deployment` label, and its managed instance groups (MIGs);
- the VPC networks and subnetworks in the deployment's region, including
  Shared VPC networks from a host project (if Cluster Director rejects a
  cross-project network, `gcluster` logs a warning and exports the rest of the
  cluster without that network entry);
- Cloud Storage buckets, Filestore instances, and Managed Lustre instances
  (internal staging buckets created by compute, scheduler, and startup-script
  modules are skipped).

For Slurm deployments, the controller, login, and compute VMs are exported as
Compute Engine instances. The Slurm configuration itself, such as partitions
and node sets, isn't sent to Cluster Director.

The export doesn't add or change labels on your VMs or instance templates, so
turning it on or off never recreates any VMs.

GKE blueprints are skipped: exporting GKE clusters isn't supported yet.

## Prerequisites

1. Enable the Cluster Director API in your project and grant your account a role
   that can create, update, and delete Cluster Director clusters (such as
   `roles/hypercomputecluster.admin` or `roles/hypercomputecluster.editor`; see
   [Before you begin](https://docs.cloud.google.com/cluster-director/docs/create/quickstart#before-you-begin)):

   ```bash
   gcloud services enable hypercomputecluster.googleapis.com --project=<PROJECT_ID>
   ```

1. `gcluster` calls the Cluster Director API with your Application Default
   Credentials and bills the calls to `project_id`, so your account also needs
   permission to use services in that project (`serviceusage.services.use`,
   included in `roles/serviceusage.serviceUsageConsumer`, Editor, and Owner):

   ```bash
   gcloud auth application-default login
   ```

## Turn on export

Set `export_to_cluster_director: true` at the top level of your blueprint or
deployment file:

```yaml
blueprint_name: my-cluster
export_to_cluster_director: true

vars:
  project_id: <PROJECT_ID>
  deployment_name: my-cluster
  region: us-central1
  zone: us-central1-a
```

Or pass the flag to `gcluster deploy`:

```bash
./gcluster deploy my-blueprint.yaml --export-to-cluster-director=true
```

When the setting is in more than one place, `gcluster` uses the first of:

1. the `--export-to-cluster-director` flag on `gcluster deploy`;
1. `export_to_cluster_director` in the deployment file (`-d`);
1. `export_to_cluster_director` in the blueprint;
1. the setting or export saved in the existing deployment folder (for example
   across `-w`);
1. the default, `false`.

The flag is saved in the deployment folder (and preserved across `-w`
overwrites when your blueprint doesn't set `export_to_cluster_director`), so
later runs of `gcluster deploy` and `gcluster destroy` keep using it until you
set `export_to_cluster_director: false` or pass
`--export-to-cluster-director=false`.

The flag can't be combined with `--only` or `--skip`: the exported cluster
always reflects every deployed group, so turning the export on or off applies
to the whole deployment.

## Deploy and export

```bash
./gcluster deploy my-blueprint.yaml --export-to-cluster-director=true
```

After Terraform finishes, `gcluster` exports the cluster, waits for the
Cluster Director operation to complete, checks the discovered nodes, and prints
a link to the cluster in the Google Cloud console:

```text
Exporting deployment "slurm-demo" to Cluster Director as cluster "slurmdemo"...
Exported cluster "slurmdemo" to Cluster Director.
Cluster Director: discovered 4 compute node(s) in cluster "slurmdemo".
View cluster in Cluster Director: https://console.cloud.google.com/cluster-director/clusters/details/us-central1/slurmdemo/overview?project=my-project
```

To export a deployment that is already deployed, run:

```bash
./gcluster deploy <DEPLOYMENT_DIR> --export-to-cluster-director=true
```

## Find your cluster

Cluster Director cluster IDs are at most 10 lowercase letters and digits, so
`gcluster` derives the ID from your `deployment_name`. Long names are shortened
(for example, `ctk-cluster-director-demo` becomes `ctkclua77e`), and Slurm
deployments may get a `cd` suffix so the ID doesn't collide with a MIG name.
Two different deployment names can map to the same ID; `gcluster` checks the
`ghpc_deployment` label before it updates or deletes a cluster, and refuses to
touch a cluster that belongs to another deployment.

To find the cluster for a deployment:

- In the console, open **Cluster Director > Clusters** and filter by the label
  `ghpc_deployment` with your `deployment_name` as the value. To list every
  cluster exported by Cluster Toolkit, filter by
  `imported_from_cluster_toolkit = true`.
- The cluster's **Description** shows `Cluster Toolkit deployment: <deployment_name>`.
- `<DEPLOYMENT_DIR>/.ghpc/artifacts/cluster-director-export.json` records the
  exported cluster ID.

Every exported cluster has these labels:

| Label | Value |
|---|---|
| `imported_from_cluster_toolkit` | `true` |
| `ghpc_deployment` | your `deployment_name` |
| `ghpc_blueprint` | your `blueprint_name` |

Other global labels of the blueprint, such as your own `vars.labels`, are copied
to the cluster too.

## Update an exported cluster

When you change the blueprint and run `gcluster deploy` again, `gcluster`
updates the exported cluster in place if its resources or labels changed, and
does nothing if they didn't.

For blueprints with several deployment groups:

- `gcluster deploy <DEPLOYMENT_DIR> --only <GROUP>` exports every group deployed
  so far.
- `gcluster destroy <DEPLOYMENT_DIR> --only <GROUP>` removes that group's
  resources from the exported cluster. When no groups are left, the exported
  cluster is deleted.

## Stop exporting

To delete the exported cluster from Cluster Director but keep all your
infrastructure running:

```bash
./gcluster deploy <DEPLOYMENT_DIR> --export-to-cluster-director=false
```

You can export it again later with `--export-to-cluster-director=true`.

## Destroy

`gcluster destroy <DEPLOYMENT_DIR>` destroys the Terraform deployment groups
first. When that succeeds, it deletes the exported cluster from Cluster
Director.

To delete an exported cluster by hand, for example after removing the
deployment folder, run:

```bash
gcloud alpha cluster-director clusters delete <CLUSTER_ID> --project=<PROJECT_ID> --location=<REGION>
```

## Limitations

- Exporting to Cluster Director is in preview and uses the `v1alpha` API, so
  details of the export can change.
- GKE blueprints aren't exported.
- Only subnetworks in the deployment's region are exported. Subnetworks in
  other regions are skipped with a warning.
- Cluster Toolkit owns the exported cluster's labels and description: changes
  made to them in the Cloud console are replaced on the next `gcluster deploy`.
- Instances are selected by the `ghpc_deployment` label across the whole
  project, so use a `deployment_name` that is unique in the project.

## Local artifacts

An exported deployment writes two files to `<DEPLOYMENT_DIR>/.ghpc/artifacts/`:

- `cluster-director-export.json`: the exported cluster ID, deployment name,
  project, and region. `gcluster` uses it to update or delete the cluster later.
- `cluster-director-request.json`: the request sent to Cluster Director, for
  troubleshooting.

## Troubleshooting

If the export fails, `gcluster deploy` exits with an error, but your
infrastructure stays deployed: nothing is rolled back. Fix the cause and retry
the export with:

```bash
./gcluster deploy <DEPLOYMENT_DIR> --export-to-cluster-director=true
```

| Message | Cause | Fix |
|---|---|---|
| `The Cluster Director API is not enabled in project ...` | The API is disabled. | Run the `gcloud services enable` command that `gcluster` prints. |
| `Your account lacks serviceusage.services.use on quota project ...` | Your account can't bill API calls to `project_id`. | Grant `roles/serviceusage.serviceUsageConsumer` on `project_id`. |
| `Permission denied in project ...` | Your account can't manage Cluster Director clusters, or your project doesn't have access to importing existing clusters. | Check your IAM roles (`roles/hypercomputecluster.admin` or `roles/hypercomputecluster.editor`) and your project's access. |
| `Your Application Default Credentials are missing or expired` | No valid ADC token. | Run `gcloud auth application-default login`. |
| `could not read remote Terraform state for group ...` | A group configures a remote Terraform backend (such as a Cloud Storage bucket) that could not be initialized or read. | Check that the state bucket exists and your credentials can access it, or run `terraform -chdir=<DEPLOYMENT_DIR>/<GROUP> init` to inspect the error. |
| `cluster "<ID>" in Cluster Director belongs to deployment "<OTHER>", not "<YOURS>"` | Another deployment in the same project and region already uses this cluster ID. | Use a different `deployment_name`, or delete the other cluster first. |
| `cluster "<ID>" currently reports 0 compute nodes` | Newly created VMs take a few minutes to appear, the VMs aren't in `vars.region`, or Slurm dynamic partitions are scaled to zero. | Wait a few minutes and check the console, or verify that instances are running in zones of `vars.region`. |

If `gcluster destroy` can't delete or update the exported cluster after
removing the infrastructure, it logs a warning (preserving the local marker and
printing the command to delete the cluster by hand) rather than failing the
destroy.
