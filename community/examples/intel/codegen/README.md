# Cluster Toolkit Examples

This repository provides various example configurations for deploying and managing workloads using GKE. The focus is on leveraging Intel optimizations for workload execution. Below is a detailed description of the `codegen.yaml` example file. 

## `codegen.yaml`

The `codegen.yaml` file contains all the necessary details specific to Codegen deployment on GKE environment.

### Configuration Details

The `codegen.yaml` file includes the following components: 

-   **Deployment**: Specifies the number of replicas and the container configuration.
-   **Service**: Exposes the application for external access.
-   **Resource Requests and Limits**: Ensures efficient usage of Intel hardware capabilities.

### Deployment Instructions

1. **Clone the repository**:
   ```bash
   git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
   cd cluster-toolkit

1. **Deploy Cluster Toolkit**:
   ./gcluster deploy community/examples/intel/codegen/codegen-cluster.yaml

1.  **Verify the deployment, pods and services in the default namespace**:

    `kubectl get pods`
    `kubectl get services`
    `kubectl get deployments`

    Ensure all pods, services and deployments are in the `Running` state.

### Contributing

Contributions to the repository are welcome. If you have improvements or additional examples, feel free to submit a pull request.

# Using Sample Manifests for Codegen (v1.1)

The sample manifests are pulled from the following location:

> [codegen.yaml (v1.1)](https://github.com/opea-project/GenAIExamples/blob/v1.1/CodeGen/kubernetes/intel/cpu/xeon/manifest/codegen.yaml)

> **Note**: We are deploying version **1.1**, so make sure to use the **v1.1** branch or tag for consistency.

### For Testing Codegen ###

1\.  **Port-forward the service:**

kubectl port-forward svc/codegen 7778:7778

1\.  **Run a test request:**

curl  http://localhost:7778/v1/codegen  
     -H  "Content-Type:  application/json"  
     -d  '{  "messages":  "Generate  API  code  for  a  TODO  list."  }'

### For Support ###
