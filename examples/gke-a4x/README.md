## Deployment Instructions

1. Create a Google Cloud Workstation. If you are facing access issues, reach out to kamesh-team@google.com.
1. Clone the Cluster Toolkit repository.

        git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
1. Build Cluster Toolkit.

        cd cluster-toolkit && git checkout main && make
1. Create a Cloud Storage bucket to store the state of the Terraform deployment:

        gcloud storage buckets create gs://BUCKET_NAME \
            --default-storage-class=STANDARD \
            --project=PROJECT_ID \
            --location=COMPUTE_REGION_TERRAFORM_STATE \
            --uniform-bucket-level-access
        gcloud storage buckets update gs://BUCKET_NAME --versioning
    Replace the following variables:

    BUCKET_NAME: the name of the new Cloud Storage bucket.

    PROJECT_ID: your Google Cloud project ID.

    COMPUTE_REGION_TERRAFORM_STATE: the compute region where you want to store the state of the Terraform deployment.
1. Modify the vars section as per your preference.
    1. DEPLOYMENT_NAME: a unique name for the deployment. If the deployment name isn't unique within a project, cluster creation fails.
    1. BUCKET_NAME: the name of the Cloud Storage bucket you created in the previous step.
    1. PROJECT_ID: your Google Cloud project ID.
    1. COMPUTE_REGION: the compute region for the cluster.
    1. COMPUTE_ZONE: the compute zone for the node pool of A4X machines.
    1. STATIC_NODE_COUNT: the number of A4X nodes in your cluster.
    1. IP_ADDRESS/SUFFIX: The IP address range that you want to allow to connect with the cluster. This CIDR block must include the IP address of the machine to call Terraform.
    1. For the extended_reservation field, use one of the following, depending on whether you want to target specific blocks in a reservation when provisioning the node pool:

        To place the node pool anywhere in the reservation, provide the name of your reservation (RESERVATION_NAME).
        To target a specific block within your reservation, use the reservation and block names in the following format:

            RESERVATION_NAME/reservationBlocks/BLOCK_NAME
    1. SYSTEM_NODE_POOL_DISK_SIZE_GB: the size of disk for each node of the system node pool. The default value is 100 GB.
    1. A4X_NODE_POOL_DISK_SIZE_GB: the size of disk for each node of the A4X node pool. The default value is 100 GB.
1. Authenticate gcloud.

        gcloud auth application-default login
1. Run gcluster deploy to deploy the infra. Hit ‘a’ to apply changes, or ‘d’ to view the Terraform plan.

        cd ~/cluster-toolkit
        ./gcluster deploy -d \
        examples/gke-a4x/gke-a4x-deployment.yaml \
        examples/gke-a4x/gke-a4x.yaml
