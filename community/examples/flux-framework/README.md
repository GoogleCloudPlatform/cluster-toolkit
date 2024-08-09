## [flux-framework](https://flux-framework.org/) Cluster

The [flux-cluster.yaml](flux-cluster.yaml) blueprint describes a flux-framework cluster where flux
is deployed as the native resource manager as described in the [Flux Administrator's Guide](https://flux-framework.readthedocs.io/en/latest/guides/admin-guide.html).

The cluster includes

- A management node
- A login node
- Four compute nodes each of which is an instance of the c2-standard-16 machine type

> **_NOTE:_** prior to running this Cluster Toolkit example the [Flux Framework GCP Images](https://github.com/GoogleCloudPlatform/scientific-computing-examples/tree/main/fluxfw-gcp/img#flux-framework-gcp-images)
> must be created in your project.

### Initial Setup for flux-framework Cluster

Before provisioning any infrastructure in this project you should follow the
Toolkit guidance to enable [APIs][apis] and establish minimum resource
[quotas][quotas]. In particular, the following APIs should be enabled

- [compute.googleapis.com](https://cloud.google.com/compute/docs/reference/rest/v1#service:-compute.googleapis.com) (Google Compute Engine)
- [secretmanager.googleapis.com](https://cloud.google.com/secret-manager/docs/reference/rest#service:-secretmanager.googleapis.com) (Secret manager, for secure mode)

[apis]: ../../../README.md#enable-gcp-apis
[quotas]: ../../../README.md#gcp-quotas

### Deploy the flux-framework Cluster

Use `gcluster` to provision the blueprint

```bash
gcluster create community/examples/flux-framework --vars project_id=<<PROJECT_ID>>
```

This will create a directory containing Terraform modules.

Follow `gcluster` instructions to deploy the cluster

```text
terraform -chdir=flux-fw-cluster/primary init
terraform -chdir=flux-fw-cluster/primary validate
terraform -chdir=flux-fw-cluster/primary apply
```
  
### Connect to the login node

Access the cluster via the login node from the command line.

```bash
gcloud compute ssh gfluxfw-login-001
```

Or via the Google Cloud Console

1. Open the following URL in a new tab.

   https://console.cloud.google.com/compute

   This will take you to **Compute Engine > VM instances** in the Google Cloud Console.

   Select the project in which the flux-framework cluster was provisioned.

2. Click on the **SSH** button associated with the **gfluxfw-login-001** instance to open a window with a terminal into the cluster login node.

### Verify the flux-framework Cluster

View the cluster resources

```bash
flux resource list
```

The output will look similar to

```text
     STATE PROPERTIES NNODES   NCORES NODELIST
      free x86-64,e2       1        2 gfluxfw-login-001
      free x86-64,c2       4       32 gfluxfw-compute-[001-004]
 allocated                 0        0 
      down                 0        0 
```

Run a simple job that executes the `hostname` command on each of the cluster compute nodes

```bash
flux run -N4 --requires=c2 hostname
```

The output will be something like

```text
gfluxfw-compute-001
gfluxfw-compute-004
gfluxfw-compute-003
gfluxfw-compute-002
```

Create a two node allocation

```bash
flux alloc -N2 --requires=c2
```

View the resources associated with the allocation

```bash
flux resource list
```

The output will look similar to

```text
     STATE PROPERTIES NNODES   NCORES NODELIST
      free x86-64,c2       2       16 gfluxfw-compute-[003-004]
 allocated                 0        0 
      down                 0        0 
```

Observe the impact on cluster resources

```bash
flux --parent resource list
```

Yields output like

```text
     STATE PROPERTIES NNODES   NCORES NODELIST
      free x86-64,e2       1        2 gfluxfw-login-001
      free x86-64,c2       2       16 gfluxfw-compute-[001-002]
 allocated x86-64,c2       2       16 gfluxfw-compute-[003-004]
      down                 0        0 
```

Use `^d` to release the resources in the allocation and return to the login node.

### Next Steps

To learn how to make the best use of flux follow the [Introduction to Flux](https://hpc-tutorials.llnl.gov/flux/)
tutorial.
