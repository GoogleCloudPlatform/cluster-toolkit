## Completed Migration to Slurm-GCP v6

[Slurm-GCP](https://github.com/GoogleCloudPlatform/slurm-gcp) is the set of
scripts and tools that automate the installation, deployment, and certain
operational aspects of [Slurm](https://slurm.schedmd.com/overview.html) on
Google Cloud Platform. The Cluster Toolkit team has finished transitioning to
Slurm-GCP v6 and has removed all v5 modules and blueprints. Slurm-GCP v6 is the
only supported option for provisioning Slurm on Google Cloud.

### Major Changes in from Slurm GCP v5 to v6

* Robust reconfiguration

  Reconfiguration is now managed by a service that runs on each instance. This has removed the dependency on the Pub/Sub Google cloud service, and provides a more consistent reconfiguration experience (when calling `gcluster deploy blueprint.yaml -w`). Reconfiguration has also been enabled by default.

* Faster deployments

  Simple cluster deploys up to 3x faster.

* Lift the restriction on the number of deployments in a single project.

  Slurm GCP v6 has eliminated the use of project metadata to store cluster configuration. Project metadata was both slow to update and had an absolute storage limit. This restricted the number of clusters that could be deployed in a single project. Configs are now stored in a Google Storage Bucket.

* Fewer dependencies in the deployment environment

  Reconfiguration and compute node cleanup no longer require users to install local python dependencies in the deploy
ent environment (where gcluster is called). This has allowed for these features to be enabled by default.

* Flexible node to partition relation

  The v5 concept of "node-group" was replaced by "nodeset" to align with Slurm naming convention. Nodeset can be attr
buted to multiple partitions, as well as partitions can include multiple nodesets.

* Upgrade Slurm to 23.11
* TPU v3, v4 support

### Unsupported use of End-of-Life modules

### v5

The final release of Slurm-GCP v5 was made as part of
[Cluster Toolkit v1.44.1][v1.44.1]. Any remaining use of Slurm-GCP v5 is
unsupported, however this release can be used to build the Toolkit binary
and review v5 modules and examples as references.

### v4

The final release of Slurm-GCP v4 was made as part of
[Cluster Toolkit v1.27.0][v1.27.0]. Any remaining use of Slurm-GCP v4 is
unsupported, however this release can be used to build the Toolkit binary
and review v4 modules and examples as references.

[v1.27.0]: https://github.com/GoogleCloudPlatform/hpc-toolkit/releases/tag/v1.27.0
[v1.44.1]: https://github.com/GoogleCloudPlatform/hpc-toolkit/releases/tag/v1.44.1
