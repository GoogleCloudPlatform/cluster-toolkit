# Google Cluster Toolkit Open Front End

This is a web front-end for HPC applications on GCP. It delegates to the Cloud
Cluster Toolkit to create cloud resources for HPC clusters. Through the convenience
of a web interface, system administrators can manage the life cycles of HPC
clusters and install applications; users can prepare & submit HPC jobs and run
benchmarks. This web application is built upon the Django framework.

## Deployment

This system can be deployed on GCP by an administrator using the following
steps:

* Arrange a hosting GCP project for this web application.
* Prepare the client side environment and secure sufficient IAM permissions for
  the system deployment.
* When ready, clone this repository and run the deployment script at
  `cluster-toolkit/community/front-end/ofe/deploy.sh` from a client machine or a Cloud
  Shell. Follow instructions to complete the deployment. The whole process is
  automated via Terraform and should complete within 15 minutes.
* Perform post-deployment configurations.

Please visit the [Administrator's Guide](docs/admin_guide.md) for more
information on system deployment.

Once the deployment is done, the administrator can use the web interface to
create HPC clusters, install applications, and set up other users. More
information is available in the [Administrator's Guide](docs/admin_guide.md)
and [User Guide](docs/user_guide.md).

You are welcome to contribute to this project. The
[Developer's Guide](docs/developer_guide.md) contains more information on the
implementation details of the system.
