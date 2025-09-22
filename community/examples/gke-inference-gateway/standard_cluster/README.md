# GKE Inference Gateway Example

This directory contains an example blueprint for deploying a GPU-based inference server on Google Kubernetes Engine (GKE) using the [GKE Inference Gateway](https://cloud.google.com/kubernetes-engine/docs/how-to/serve-llms-with-inference-gateway).

This blueprint deploys the `meta-llama/Llama-3.1-8B-Instruct` model using the vLLM model server.

## Prerequisites

1.  A Google Cloud project with billing enabled.
2.  Sufficient quota for `H100_80GB_GPU` accelerators in your chosen region.
3.  A Hugging Face account and an access token with permission to access the `meta-llama/Llama-3.1-8B-Instruct` model.
4.  The `gcloud` CLI and `git` installed and authenticated.
5.  A local clone of the `cluster-toolkit` repository.

## Deployment

1.  **Navigate to the root of the repository:**
    ```sh
    cd /path/to/cluster-toolkit
    ```

2.  **Set environment variables:**
    Replace the placeholder values with your specific information.
    ```sh
    export GOOGLE_CLOUD_PROJECT="your-gcp-project-id"
    export HUGGING_FACE_TOKEN="your-hf-token"
    export AUTHORIZED_IP=$(curl -s ifconfig.me)/32
    ```
    **Note:** Using `ifconfig.me` will authorize the public IP address of the machine you are running the command from to access the GKE cluster's control plane. Adjust if you are behind a NAT or need a different IP range.

3.  **Deploy the blueprint:**
    Run the `gcluster` command to deploy the resources. This command will create a VPC, a GKE cluster with a GPU node pool, and deploy the necessary Kubernetes manifests for the inference gateway and model server.
    ```sh
    ./gcluster deploy community/examples/gke-inference-gateway/standard_cluster/gke-inference-gateway.yaml \
      --vars project_id=$GOOGLE_CLOUD_PROJECT \
      --vars hf_token=$HUGGING_FACE_TOKEN \
      --vars authorized_cidr=$AUTHORIZED_IP \
      -o ~/cluster_toolkit_output -w
    ```
    The deployment process will take several minutes.

## Updating Manifests

The Kubernetes manifests included in the `manifests/` directory are sourced from the official `gateway-api-inference-extension` repository. To ensure you have the latest versions, you can run the provided update script:

```sh
./community/examples/gke-inference-gateway/manifests/update-manifests.sh
```
This will pull the latest manifests from the upstream repository and overwrite the local files.

## Cleanup

To destroy all the resources created by this blueprint, run the `gcluster` destroy command, pointing to the same output directory:

```sh
./gcluster destroy ~/cluster_toolkit_output
```

## Troubleshooting

### Deployment Fails with Kubernetes Connection Errors

During the initial deployment, you may encounter errors such as `Kubernetes cluster unreachable`, `connection timed out`, or `resource [...] isn't valid for cluster`.

**Cause:** This is typically a network race condition. The `gcluster` tool creates the GKE cluster and immediately attempts to apply Kubernetes resources (like CRDs, Gateways, and Deployments) to it. However, the firewall rule that authorizes your IP address to access the cluster's control plane may not have fully propagated.

When the tool cannot connect to the cluster, it cannot verify that the necessary Custom Resource Definitions (CRDs) are installed. This leads to the misleading but related error that resources like `Gateway` or `HTTPRoute` "aren't valid for the cluster," when in fact the real issue is the lack of connectivity.

**Solution:** The fix is to simply **re-run the exact same `./gcluster deploy` command**. By the time you run it again, the network rules will have propagated. Terraform will read its state file, see that the GKE cluster already exists, and successfully connect to apply the remaining Kubernetes resources.
