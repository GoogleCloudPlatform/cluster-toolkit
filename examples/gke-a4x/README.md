## Deployment Instructions

1. Install Cluster Toolkit.

   From the CLI, complete the following steps:

   1. Install [dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).
   1. Set up [Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment).
1. Create a reservation.

   If you don't have a reservation provided by a [Technical Account Manager (TAM)](https://cloud.google.com/tam), we recommend creating a reservation. For more information, see [Choose a reservation type](https://cloud.google.com/compute/docs/instances/choose-reservation-type).

   Reservations incur ongoing costs even after the GKE cluster is destroyed. To manage your costs, we recommend the following options:
   1. Track spending by using [budget alerts](https://cloud.google.com/billing/docs/how-to/budgets).
   1. Delete reservations when you're done with them. To delete a reservation, see [delete your reservation](https://cloud.google.com/compute/docs/instances/reservations-delete).

   To create a reservation, run the `gcloud compute reservations create` [command](https://cloud.google.com/sdk/gcloud/reference/compute/reservations/create) and ensure that you specify the `--require-specific-reservation` flag.

        gcloud compute reservations create RESERVATION_NAME \
                --require-specific-reservation \
                --project=PROJECT_ID \
                --machine-type=a4x-highgpu-4g \
                --vm-count=NUMBER_OF_VMS \
                --zone=ZONE

   Replace the following:
   1. `RESERVATION_NAME`: a name for your reservation.
   1. `PROJECT_ID`: your project ID.
   1. `NUMBER_OF_VMS`: the number of VMs needed for the cluster.
   1. `ZONE`: a zone that has `a4x-highgpu-4g` machine types.
1. Create a cluster.

   Use the following instructions to create a cluster using Cluster Toolkit.
   1. After you have installed the Cluster Toolkit, ensure that you are in the Cluster Toolkit directory. To go to the main Cluster Toolkit blueprint's working directory, run the following command from the CLI.

           cd cluster-toolkit
   1. Create a Cloud Storage bucket to store the state of the Terraform deployment:

           gcloud storage buckets create gs://BUCKET_NAME \
               --default-storage-class=STANDARD \
               --project=PROJECT_ID \
               --location=COMPUTE_REGION_TERRAFORM_STATE \
               --uniform-bucket-level-access
           gcloud storage buckets update gs://BUCKET_NAME --versioning
      Replace the following variables:

      `BUCKET_NAME`: the name of the new Cloud Storage bucket.

      `PROJECT_ID`: your Google Cloud project ID.

      `COMPUTE_REGION_TERRAFORM_STATE`: the compute region where you want to store the state of the Terraform deployment.
   1. Modify the vars section as per your preference.
      1. `DEPLOYMENT_NAME`: a unique name for the deployment. If the deployment name isn't unique within a project, cluster creation fails.
      1. `BUCKET_NAME`: the name of the Cloud Storage bucket you created in the previous step.
      1. `PROJECT_ID`: your Google Cloud project ID.
      1. `COMPUTE_REGION`: the compute region for the cluster.
      1. `COMPUTE_ZONE`: the compute zone for the node pool of A4X machines.
      1. `STATIC_NODE_COUNT`: the number of A4X nodes in your cluster.
      1. `IP_ADDRESS/SUFFIX`: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine to call Terraform. For more information, see [How authorized networks work](https://cloud.google.com/kubernetes-engine/docs/concepts/network-isolation#how_authorized_networks_work). To get the IP address for your host machine, run the following command.
      1. For the extended_reservation field, use one of the following, depending on whether you want to target specific blocks in a reservation when provisioning the node pool:

          To place the node pool anywhere in the reservation, provide the name of your reservation (RESERVATION_NAME).
          To target a specific block within your reservation, use the reservation and block names in the following format:

              RESERVATION_NAME/reservationBlocks/BLOCK_NAME
      1. `SYSTEM_NODE_POOL_DISK_SIZE_GB`: the size of disk for each node of the system node pool. The default value is 100 GB.
      1. `A4X_NODE_POOL_DISK_SIZE_GB`: the size of disk for each node of the A4X node pool. The default value is 100 GB.
   1. Authenticate gcloud.

             gcloud auth application-default login
   1. Deploy the blueprint to provision the GKE infrastructure using A4X machine types. Hit ‘a’ to apply changes, or ‘d’ to view the Terraform plan.

             ./gcluster deploy -d \
             examples/gke-a4x/gke-a4x-deployment.yaml \
             examples/gke-a4x/gke-a4x.yaml
1. Clean up resources created by Cluster Toolkit

   To avoid recurring charges for the resources used on this page, clean up the resources provisioned by Cluster Toolkit, including the VPC networks and GKE cluster:

           cd ~/cluster-toolkit
           ./gcluster destroy CLUSTER_NAME/

   Replace `CLUSTER_NAME` with the name of your cluster. For the clusters created with Cluster Toolkit, the cluster names are based on the `DEPLOYMENT_NAME` name.
