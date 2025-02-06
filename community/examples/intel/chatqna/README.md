# Cluster Toolkit Examples

This repository provides various example configurations for deploying and managing workloads using Kubernetes. The focus is on leveraging Intel optimizations for workload execution. Below is a detailed description of the `chatqna.yaml` example file. 

## `chatqna.yaml`

The `chatqna.yaml` file demonstrates the deployment of a Q&A application powered by Intel hardware optimizations. This configuration is designed to run efficiently on Kubernetes clusters with Intel-specific workloads.

### Key Features

- **Optimized for Intel Hardware**: Utilizes Intel's hardware capabilities to enhance performance.
- **Scalable Architecture**: Configured for horizontal scaling to handle varying traffic loads.
- **Resource Customization**: Fine-tuned CPU and memory resource allocation for optimal performance.
- **Portable Setup**: Easily deployable on any Kubernetes cluster with minimal configuration changes.

### Prerequisites

- A Kubernetes cluster (1.22+ recommended).
- Intel-optimized nodes or Intel-specific hardware in the cluster.
- `kubectl` installed and configured to access your Kubernetes cluster.

### Configuration Details

The `chatqna.yaml` file includes the following components: 

-   **Deployment**: Specifies the number of replicas and the container configuration.
-   **Service**: Exposes the application for external access.
-   **Resource Requests and Limits**: Ensures efficient usage of Intel hardware capabilities.

### Customization

You can customize the deployment by modifying the `chatqna.yaml` file:

-   **Replica Count**: Adjust the `replicas` value in the deployment.
-   **Resource Allocation**: Modify `resources.requests` and `resources.limits` to match your workload requirements.
-   **Environment Variables**: Add or update environment variables for the application under `env`.

### Deployment Instructions

1. **Clone the repository**:
   ```bash
   git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
   cd cluster-toolkit

1. **Deploy Cluster Toolkit**:
   ./gcluster deploy community/examples/intel/chatqna/chatqna-cluster.yaml

1.  **Verify the deployment, pods and services in the default namespace**:

    `kubectl get pods`
    `kubectl get services`
    `kubectl get deployments`

    Ensure all pods, services and deployments are in the `Running` state.

### Contributing

Contributions to the repository are welcome. If you have improvements or additional examples, feel free to submit a pull request.

# Using Sample Manifests for ChatQnA (v1.1)

The sample manifests are pulled from the following location:

> [chatqna.yaml (v1.1)](https://github.com/opea-project/GenAIExamples/blob/v1.1/ChatQnA/kubernetes/intel/cpu/xeon/manifest/chatqna.yaml)

> **Note**: We are deploying version **1.1**, so make sure to use the **v1.1** branch or tag for consistency.

### For Testing ChatQNA###

1\.  **Port-forward the service:**

kubectl port-forward svc/chatqna 8888:8888

1\.  **Run a test request:**

curl http://localhost:8888/v1/chatqna

    -H 'Content-Type: application/json'

    -d '{"messages": "What is the revenue of X in 2023?"}'

### For Support ###

For any issues, please contact `info@opea.dev`
