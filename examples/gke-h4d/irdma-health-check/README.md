# iRDMA Health Check Mutating Webhook for GKE

This document provides instructions for setting up a mutating webhook in a GKE cluster to automatically run an iRDMA health check on H4D nodes.

## Overview

The solution consists of the following components:

1. **Health Check Container**: A container image based on Rocky Linux 8 that contains a script to check the iRDMA device status.
1. **Mutating Webhook**: A Go application that runs in the cluster and injects the health check container as an init container into pods *on creation* that have the `nodeSelector` `node.kubernetes.io/instance-type: h4d-highmem-192-lssd`.
1. **Kubernetes Manifests**: A set of YAML files to deploy the webhook and its dependencies.

## Prerequisites

- A GKE cluster with H4D nodes.
- `gcloud`, `docker`, and `kubectl` CLIs installed and configured.
- `cert-manager` installed in your cluster. If you don't have it, install it by running:
  ```plaintext
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.11.0/cert-manager.yaml
  ```
  **Note**: After applying the manifest, wait for the `cert-manager` pods to be in the `Running` state before proceeding. You can check the status with the following command:
  ```plaintext
  kubectl get pods -n cert-manager
  ```

## Setup and Deployment

**Note**: Before you begin, please change into the directory containing the solution files:
```plaintext
cd examples/gke-h4d/irdma-health-check
```

### 1. Customization
Before proceeding, you need to customize the following values in the `build-and-push.sh` and `build-and-push-webhook.sh files`:
 
*   **`PROJECT_ID`**: Your Google Cloud project ID.
*   **`IMAGE_NAME`**: The name for the Docker image (e.g., `irdma-health-check`, `irdma-webhook-server`).
*   **`IMAGE_TAG`**: The version tag for your Docker images (e.g., `v1.0.0`).
*   **`REGION`**: The Google Cloud region where your Artifact Registry is located (e.g., `us-central1`).


### 2. Build and Push the Health Check Init Container Image

The health check script and its Dockerfile are provided.

1.  **Run the build script**:
    ```sh
    ./build-and-push.sh
    ```
This will build the Docker image (`us-central1-docker.pkg.dev/MY-GCP-PROJECT/h4d/irdma-health-check:v1.0.0`) and push it to your project's Google Container Registry.

### 3. Build and Push the Webhook Server Image

The webhook server Go application and its Dockerfile are provided in the `webhook/` directory.

1.  **Verify Health Check Init Container Image**: Before applying, ensure the `imageURI` field in `webhook/main.go` matches the URI of the `irdma-health-check` image you pushed (e.g., `us-central1-docker.pkg.dev/MY-GCP-PROJECT/h4d/irdma-health-check:v1.0.0`).

1.  **Run the build script**:
    ```sh
    ./build-and-push-webhook.sh
    ```
    This will build the Docker image (`us-central1-docker.pkg.dev/MY-GCP-PROJECT/h4d/irdma-webhook-server:v1.0.0`) and push it to your project's Google Container Registry.

### 4. Deploy the Webhook

The `manifests` directory contains all the necessary Kubernetes resources.

1.  **Verify Webhook Deployment Image**: Before applying, ensure the `image` field in `manifests/04-webhook-deployment.yaml` matches the URI of the `irdma-webhook-server` image you pushed (e.g., `us-central1-docker.pkg.dev/MY-GCP-PROJECT/h4d/irdma-webhook-server:v1.0.0`).

1.  **Apply the manifests**:
    ```sh
    kubectl apply -f manifests/
    ```
    This will create:
    - A namespace `irdma-health-check`.
    - A self-signed issuer and a certificate for the webhook using `cert-manager`.
    - The webhook deployment and service.
    - The `MutatingWebhookConfiguration` that tells the Kubernetes API server to forward pod creation requests to the webhook.

### 5. Test the Webhook

A sample pod manifest `test-pod-trigger.yaml` is provided to test the webhook.

1.  **Deploy the test pod**:
    ```plaintext
    kubectl apply -f test-pod-trigger.yaml
    ```
     This pod has the `nodeSelector` `node.kubernetes.io/instance-type: h4d-highmem-192-lssd`, so the webhook will act on it. It also has tolerations to ensure it gets scheduled on an H4D node.

1.  **Verify the injection**:
    Check the pod's definition to see if the `irdma-health-check` init container was injected by the webhook:
    ```plaintext
    kubectl get pod my-h4d-app-irdma-check-rocky8 -o yaml
    ```
    You should see the `irdma-health-check` container in the `spec.initContainers` section, along with the `securityContext` and `resources` injected by the webhook.

1.  **Check the logs**:
    If the init container runs, you can check its logs:
    ```plaintext
    kubectl logs my-h4d-app-irdma-check-rocky8 -c irdma-health-check
    ```
    If the health check fails, the pod will not start, and you can see the error by describing the pod:
    ```plaintext
    kubectl describe pod my-h4d-app-irdma-check-rocky8
    ```

## How It Works

1.  **Webhook Trigger**: On *pod creation requests*, if a pod includes the `nodeSelector` `node.kubernetes.io/instance-type: h4d-highmem-192-lssd`, the Kubernetes API server, as configured by the `MutatingWebhookConfiguration`, sends an admission review request to the `irdma-webhook` service. The `MutatingWebhookConfiguration` specifies that the webhook should intercept `CREATE` operations on `pods` resources.
2.  **Webhook Logic**: The webhook server receives the request, validates the `nodeSelector`, and if present, generates a JSON patch to inject the `irdma-health-check` init container into the pod's `spec`.
3.  **Init Container Execution**: The injected init container runs before any main application containers. It executes the `irdma-health-check.sh` script.
4.  **Health Check Outcome**: The script checks the RDMA device status and performs a loopback bandwidth test.
    - If the health check fails (e.g., due to low bandwidth), the script attempts to recover the interface. If recovery is successful, it re-runs the test.
    - If the health check (or re-check after recovery) ultimately fails, the script exits with a non-zero status code, causing the init container to fail.
5.  **Pod Scheduling Impact**: If the init container fails persistently, Kubernetes will not schedule the main application containers, indicating a problem with the node's iRDMA setup.

**Important Note on Namespace Selectors**: The `MutatingWebhookConfiguration` is configured to *not* run on pods in the `irdma-health-check` and `cert-manager` namespaces. This is critical to prevent a circular dependency where the webhook tries to mutate its own pods or the `cert-manager` pods, which would cause the system to become unstable.

## Cleanup

To remove all the resources created by this example, run:
```plaintext
kubectl delete -f test-pod-trigger.yaml
kubectl delete -f manifests/
```

## Troubleshooting

1. **Error**: `denied: Permission "artifactregistry.repositories.uploadArtifacts" denied on resource "projects/hpc-topolkit-dev/locations/us-central1/repositories/h4d" (or it may not exist)`

   Run `gcloud auth configure-docker us-central1-docker.pkg.dev`
