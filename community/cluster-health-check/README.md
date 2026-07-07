# DCGM Health Check

This tool runs as a Kubernetes DaemonSet to perform passive health checks on NVIDIA GPUs within a GKE cluster using DCGM (Data Center GPU Manager) and NVML.

It continuously monitors GPU nodes for hardware errors, XID errors, and InfiniBand/network issues. When an issue is detected, the agent patches the Kubernetes Node object to report the failure, applying the appropriate severity label (`Warning`, `Failure` or `Fatal`).

## Features

- **Passive Health Checking**: Connects to the local DCGM daemon to watch for hardware errors without disrupting running workloads.
- **Failure Reporting**: Automatically adds conditions to the Kubernetes Node (e.g., `GPUUnhealthy`) and sets the `cloud.google.com/health-check-status` label to the severity of the issue (e.g., `Fatal`, `Warning`) when issues occur. The label is cleared when the node is healthy.
- **Configurable Severities**: Any detected XID error marks the node as unhealthy with a `Warning` severity. You can define specific XID errors to escalate to `Fatal` severity via a ConfigMap.

## How to Use

The health check agent is deployed as a DaemonSet to ensure it runs on every node with an NVIDIA GPU.

### Deployment

1. **Build and push the image**:
   Use the provided script to build a multi-architecture Docker image and push it to Artifact Registry:
   ```bash
   ./build-and-push-cluster-health-check.sh -p <YOUR_PROJECT> -r <YOUR_REPO> -i cluster-health-check
   ```

2. **Update the image reference**:
   Edit `deployment/daemonset.yaml` to replace the `<YOUR_REGISTRY>/<YOUR_REPO>/cluster-health-check:latest` image string with the remote destination you pushed to in step 1.

3. **Deploy the ConfigMap and DaemonSet**:
   Apply the Kubernetes manifests in the `deployment/` directory:
   ```bash
   kubectl apply -f deployment/configmap.yaml
   kubectl apply -f deployment/daemonset.yaml
   ```

### Configuration (`ConfigMap`)

The `fatal-xids-config` ConfigMap (defined in `deployment/configmap.yaml`) controls which NVIDIA XID errors escalate the node's issue severity from `Warning` to `Fatal`.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fatal-xids-config
  namespace: default
data:
  fatal-xids: "79, 119" # Comma-separated list of fatal XIDs
```

Any XID error will cause the node to be marked as unhealthy (`GPUUnhealthy=True`). If the error matches the `fatal-xids` list, the `cloud.google.com/health-check-status` label is set to `Fatal`; otherwise, it defaults to `Warning`. You can update this ConfigMap and the agent will automatically reload the configuration.
