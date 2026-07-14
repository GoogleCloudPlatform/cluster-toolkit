# DCGM Health Check

![Platform: GKE](https://img.shields.io/badge/Platform-GKE-green.svg)

This tool runs as a Kubernetes DaemonSet to perform passive health checks on NVIDIA GPUs within a GKE cluster using DCGM (Data Center GPU Manager) and NVML.

It continuously monitors GPU nodes for hardware errors, XID errors, and InfiniBand/network issues. When an issue is detected, the agent patches the Kubernetes Node object to report the failure, applying the appropriate severity label (`Warning`, `Failure` or `Fatal`).

--------------------------------------------------------------------------------

## ✨ Key Features

- **Passive Health Checking**: Connects to the local DCGM daemon to watch for hardware errors without disrupting running workloads.
- **Failure Reporting**: Automatically adds conditions to the Kubernetes Node (e.g., `GPUUnhealthy`) and sets the `cloud.google.com/health-check-status` label to the severity of the issue (e.g., `Fatal`, `Warning`) when issues occur. The label is cleared when the node is healthy.
- **Configurable XID Severities**: Any detected XID error marks the node as unhealthy with a `Warning` severity. You can define specific XID errors to escalate to `Fatal` severity via an optional ConfigMap.

## 🚀 Quick Start

The health check agent is deployed as a DaemonSet to ensure it runs on every node with an NVIDIA GPU.

### Deploying to a GKE cluster

1. **Build and push the image**:
   Use the provided script to build a multi-architecture Docker image and push it to your Artifact Registry:

   ```bash
   ./build-and-push-cluster-health-check.sh -p <YOUR_PROJECT> -r <YOUR_REPO> -i cluster-health-check
   ```

2. **Update the image reference**:
   Edit `deployment/dcgm-healthcheck.yaml` to replace the `<YOUR_REGISTRY>/<YOUR_REPO>/cluster-health-check:latest` image string with your remote destination image created in the previous step.

3. **Deploy the DaemonSet**:
   Apply the Kubernetes manifest in the `deployment/` directory:

   ```bash
   kubectl apply -f deployment/dcgm-healthcheck.yaml
   ```

### 📝 Example Output

Once deployed, the agent will continuously monitor the node's health. When a failure is detected, the agent patches the node's status conditions and metadata labels. You can view the health check results directly on the nodes:

```bash
$ kubectl get nodes -o custom-columns=NAME:.metadata.name,HEALTH:.metadata.labels.cloud\.google\.com/health-check-status
NAME                                                  HEALTH
gke-sa-gke-a4x-a4x-highgpu-4g-a4x-poo-a482f777-078q   <none>
gke-sa-gke-a4x-a4x-highgpu-4g-a4x-poo-a482f777-1rk7   warning
```

You can view the specific failure message in the node conditions using `kubectl describe`:

```bash
$ kubectl get node <node-name> -o json | jq -r '["Type", "Status", "LastTransitionTime", "Reason", "Message"], (.status.conditions[] | select(.type == "GPUUnhealthy") | [.type, .status, .lastTransitionTime, .reason, .message]) | @tsv' | column -t

Type          Status  LastTransitionTime    Reason             Message
GPUUnhealthy  True    2026-07-08T20:57:02Z  HealthCheckFailed  <detailed health check message>
```

## ⚙️ Configuration (Optional)

The `fatal-xids-config` ConfigMap is **optional**. It controls which NVIDIA XID errors escalate the node's issue severity from `Warning` to `Fatal`.

By default, any XID error will cause the node to be marked as unhealthy (`GPUUnhealthy=True`) with a `Warning` severity. If you apply this ConfigMap and an error matches the `fatal-xids` list, the `cloud.google.com/health-check-status` label will instead be set to `Fatal`.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fatal-xids-config
  namespace: default
data:
  fatal-xids: "79, 119" # Comma-separated list of fatal XIDs
```

If you wish to configure fatal XIDs, apply the ConfigMap:

```bash
kubectl apply -f deployment/configmap.yaml
```

You can update this ConfigMap at any time and the agent will automatically reload the configuration.

## 🛠️ Developer Guide: Modifying and Releasing Your Own Image

This section is for developers who wish to customize the script's behavior or release their own version of the container image to a private Google Artifact Registry.

### 1. Modifying the Code

- The core logic for generating the health check is located in the `cmd/` directory, with the main entry point being `cmd/dcgm-healthcheck/main.go`.
- The image and all relevant dependencies are defined in the `Dockerfile`.

### 2. Building and Pushing to Artifact Registry

We provide a convenient shell script to build and push your customized image to your own Artifact Registry.

You can do so by invoking the `build-and-push-cluster-health-check.sh` script with the following parameters:

| Flag | Description                                                     | Required |
| :--- | :-------------------------------------------------------------- | :------- |
| `-p` | Your Google Cloud Project ID.                                   | **Yes**  |
| `-r` | The name of your Artifact Registry repository.                  | **Yes**  |
| `-i` | The name for your image.                                        | **Yes**  |
| `-l` | The region of your Artifact Registry. Defaults to `us-central1` | No       |
| `-v` | Version tag for the image. Defaults to `YYYY-MM-DD`.            | No       |
| `-h` | Display the help message.                                       | No       |

Sample command to build and push a new image:

```bash
bash build-and-push-cluster-health-check.sh \
    -p ${PROJECT?} \
    -r ${ARTIFACT_REPO?} \
    -i "cluster-health-check" \
    -l "us-east1" \
    -v "0.0.3"
```
