# Multi-provider agentgateway on GKE

This blueprint deploys a regional, CPU-only GKE Standard cluster and a
multi-provider generative AI gateway powered by [agentgateway](https://agentgateway.dev/).
Agentgateway supports multiple GenAI API formats, including OpenAI and Anthropic.

This blueprint currently exposes OpenAI-compatible endpoints for accessing Gemini
through [Gemini Enterprise Agent Platform](https://docs.cloud.google.com/gemini-enterprise-agent-platform)
(formerly Vertex AI) using Workload Identity Federation for GKE, with optional OpenAI and Anthropic
backends configured through existing Kubernetes secrets.

```text
client -> internal Application Load Balancer -> ClusterIP Service
                                                     |
                                              agentgateway proxy
                                                |          |
                                             Gemini    OpenAI / Anthropic
```

The GKE Gateway controller manages the `agentgateway-internal` Gateway, which
uses the `gke-l7-rilb` GatewayClass, and its load balancer. The agentgateway
controller manages the `agentgateway-proxy` Gateway, which uses the `agentgateway`
GatewayClass, and its proxy Deployment.
Only agentgateway selects and authenticates to LLM providers. The agentgateway
proxy uses a `ClusterIP` Service and is exposed through an internal Application
Load Balancer, keeping the gateway private to the VPC and connected networks.

## Before you deploy

This example assumes you have completed the setup described in the
[Cluster Toolkit documentation](https://docs.cloud.google.com/cluster-toolkit).
Enable the [Gemini Enterprise Agent Platform API](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/start#configure-your-project),
confirm [model availability in the selected location](https://docs.cloud.google.com/gemini-enterprise-agent-platform/resources/locations),
and review the model's [throughput and quota requirements](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/resources/throughput-quota).
For administrator-provisioned accounts, see the [required IAM access](#use-existing-service-accounts).

Provide the public egress IP of the deployment runner as `authorized_cidr`.
Only this CIDR can reach the cluster's public API endpoint. Nodes have private
IPs and use Cloud NAT for provider and image-registry egress. Clients
must run in the VPC or a connected network in the supported load-balancer region.
Cloud Shell is not automatically connected to this VPC so use the in-cluster
validation Job described in [Run the smoke test in a Kubernetes Job](#run-the-smoke-test-in-a-kubernetes-job)
to test the real internal load-balancer path.

This example uses HTTP inside the VPC and does not authenticate downstream clients.
All clients that can reach the listener can consume the configured AI model
providers so restrict VPC access accordingly.

This blueprint does not configure external HTTPS, client authentication,
Certificate Manager, Cloud Armor, or Secret Manager CSI.

## Versions and scope

The configuration retains agentgateway's `vertexai` provider field and the
blueprint's `vertex_model` and `vertex_location` variables. The API service name
remains `aiplatform.googleapis.com`.

| Component | Configuration |
| --- | --- |
| GKE | `1.35.` version prefix, Regular release channel |
| Gateway API | GKE-managed Standard CRDs. A v1.4–1.6 CRD bundle is required. |
| agentgateway CRDs and controller | Both Helm charts use the blueprint’s `agentgateway_version` |
| Readiness and smoke client | `python:3.12.12-slim` |
| Gemini model | Configurable model and location; defaults `gemini-2.5-flash`, `global` |

Review the [agentgateway compatibility matrix](https://agentgateway.dev/docs/kubernetes/latest/release-notes/versions/)
before changing versions.

To use a different Gemini model, set `vertex_model` to its model ID and set
`vertex_location` to a location that supports it. Confirm that your project has
access and sufficient quota for that model.

GKE owns the Standard Gateway API CRDs. Do not apply upstream Gateway API CRDs
on top of them. Gateway API Inference Extension is disabled because this
blueprint uses external model APIs and does not deploy model-serving workloads
in the cluster.

Set `agentgateway_version` in the blueprint to update both Helm releases,
using a version compatible with the cluster's Gateway API bundle.

## Deploy

Choose a unique deployment name of at most 20 characters (also used for IAM and
firewall names). When creating service accounts, avoid
recently deleted names: Google Cloud soft-deletes custom IAM roles and service
accounts.

Deployment proceeds through infrastructure, controller installation, gateway
configuration, and ingress, waiting for each stage to become ready.
During cluster creation, the GKE module replaces the initial default node pool
with the configured system pool; see
[Node pool lifecycle](../../../modules/scheduler/gke-cluster/README.md#node-pool-lifecycle).

The system node pool scales automatically between one and two total
`e2-standard-4` nodes based on Pod scheduling needs. Each node has a 50 GB disk.
This is not a high-availability production deployment. Costs also include the
regional cluster, internal application load balancer, NAT, logging/metrics and
model inference requests. Destroy the deployment when finished.

Choose one of the following deployment options. Run commands from the repository
root.

### Create service accounts

With the default `create_service_accounts: true`, the blueprint creates the node
and proxy service accounts, the prediction role, and their IAM bindings.

By default, the blueprint assigns the proxy's Google service account a
[custom IAM role](../../modules/project/vertex-ai-prediction/README.md) containing
only the [aiplatform.endpoints.predict permission](https://docs.cloud.google.com/iam/docs/roles-permissions/aiplatform#aiplatform.endpoints.predict),
which allows it to make model prediction requests. The proxy authenticates
through Workload Identity, so the blueprint creates no service-account key.
Node logging/monitoring permissions are assigned to a separate service account.

```sh
./gcluster create community/examples/agentgateway-gke/agentgateway-gke.yaml \
  --vars project_id=YOUR_PROJECT,deployment_name=agw-demo,authorized_cidr=YOUR_IP/32
./gcluster deploy agw-demo
```

### Use existing service accounts

If an administrator manages IAM for your project, complete the
[administrator setup](#administrator-setup), set `create_service_accounts: false`,
and provide both account emails:

```sh
./gcluster create community/examples/agentgateway-gke/agentgateway-gke.yaml \
  --vars project_id=YOUR_PROJECT,deployment_name=agw-demo,authorized_cidr=YOUR_IP/32 \
  --vars create_service_accounts=false \
  --vars node_service_account_email=nodes@YOUR_PROJECT.iam.gserviceaccount.com \
  --vars proxy_service_account_email=proxy@YOUR_PROJECT.iam.gserviceaccount.com
./gcluster deploy agw-demo
```

This mode creates no service accounts, custom IAM roles, or IAM bindings. It uses
these identities for the Gemini backend, including the proxy's Workload
Identity annotation. Cleanup leaves the existing accounts and permissions intact.
Missing or malformed account inputs fail Terraform planning before deployment.
Leave the email inputs empty when `create_service_accounts` is true.

#### Administrator setup

Before deployment, an administrator must enable the IAM Service Account
Credentials API (`iamcredentials.googleapis.com`) and configure separate node
and proxy accounts in the deployment project:

| Identity | Required access |
| --- | --- |
| Node service account | Project roles `roles/logging.logWriter`, `roles/monitoring.metricWriter`, `roles/monitoring.viewer`, `roles/stackdriver.resourceMetadata.writer`, and `roles/artifactregistry.reader` |
| Proxy service account | `aiplatform.endpoints.predict` on the deployment project, through a custom or other suitable existing role |
| Kubernetes service account | `roles/iam.workloadIdentityUser` on the proxy Google service account, granted to `serviceAccount:YOUR_PROJECT.svc.id.goog[agentgateway-system/agentgateway-proxy]` |
| Deploying identity | `iam.serviceAccounts.actAs` on the node account, for example through `roles/iam.serviceAccountUser`, in addition to the usual GKE and network deployment permissions |

The administrator can use the
[Prediction IAM module](../../modules/project/vertex-ai-prediction/README.md)
to provision the narrow prediction role separately. A role ID is not a blueprint
input: the role must already be assigned to the proxy account. Existing roles may
grant more permissions than the blueprint's default custom role.

For the Workload Identity binding, the administrator can run:

```sh
gcloud iam service-accounts add-iam-policy-binding \
  proxy@YOUR_PROJECT.iam.gserviceaccount.com \
  --role=roles/iam.workloadIdentityUser \
  --member='serviceAccount:YOUR_PROJECT.svc.id.goog[agentgateway-system/agentgateway-proxy]'
```

If you override `namespace`, use that namespace in the binding. The Kubernetes
service-account name remains `agentgateway-proxy`. See GKE's documentation for
[Workload Identity](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/workload-identity#kubernetes-sa-to-iam)
and [service-account attachment](https://docs.cloud.google.com/iam/docs/service-accounts-actas).
The blueprint validates input values; it cannot establish that all required
permissions and bindings exist until the deployment and request checks run.

Choose the identity mode before the first deployment. Do not switch an existing
managed deployment to this mode without a Terraform state migration: Terraform
would otherwise plan to delete the accounts, role, and bindings it managed.

## Validate and use

The `smoke-test.py` client performs the same request and response checks in both
options below. Run it directly from a client with VPC connectivity, or use
`run-in-cluster.py` to launch it in a temporary Kubernetes Job. The runner also
checks Kubernetes configuration and handles test resource cleanup.

Both options require Python >= 3.10 and `kubectl`. Run the commands from
the repository root, using your deployment name, region and project:

```sh
gcloud container clusters get-credentials agw-demo \
  --region us-central1 --project YOUR_PROJECT
```

Use your configured `namespace` and `vertex_model` if they differ from the defaults
shown below.

### Run the smoke test in a Kubernetes Job

From your local machine, run the Kubernetes runner to create a temporary client Job inside
the cluster:

```sh
python3 tools/cloud-build/daily-tests/ansible_playbooks/test-validation/agentgateway/run-in-cluster.py \
  --namespace agentgateway-system \
  --model gemini-2.5-flash
```

Your machine needs Kubernetes API access, with its public egress IP allowed by
`authorized_cidr`, and permission to create the test resources. It does not need
network access to the internal load balancer: the Job sends requests from inside
GKE. The `authorized_cidr` setting controls Kubernetes API access; it does not
provide access to the internal gateway.

The helper checks private exposure, synchronous and streaming Gemini responses,
and proxy metrics. It prints results and removes its temporary resources,
including on failure, leaving the deployment running. A successful run exits
with status `0`. This command uses only Python's standard library and does not
require Ansible or additional Python packages.

See [Validate an existing deployment](TESTING.md#validate-an-existing-deployment)
for synthetic-provider testing and failure diagnostics.

### Run from a client connected to the VPC

From a client with network access to the gateway's internal IP address, run the
smoke test directly to check synchronous and streaming responses:

```sh
GATEWAY_IP=$(kubectl -n agentgateway-system get gateway agentgateway-internal \
  -o jsonpath='{.status.addresses[0].value}')
python3 community/examples/agentgateway-gke/smoke-test.py \
  --endpoint "http://${GATEWAY_IP}" --model gemini-2.5-flash
```

Send OpenAI-compatible requests to `/v1/chat/completions`. The configured Gemini
model takes precedence over the request model. Use `stream: true` for SSE. Both
the internal load balancer and proxy routes use 300-second request timeouts.

## Optional external providers

For provider setup examples, see agentgateway's
[OpenAI integration guide](https://agentgateway.dev/docs/kubernetes/latest/integrations/llm/providers/openai/)
and [Anthropic integration guide](https://agentgateway.dev/docs/kubernetes/latest/integrations/llm/providers/anthropic/).
Follow the steps below to configure these providers in this blueprint.

Leave both secret-name variables empty for a Gemini-only deployment. To enable
an external provider, first deploy the default blueprint, then create an Opaque
secret in `agentgateway-system` outside Terraform. The secret needs an
`Authorization` entry containing the provider API key (a `Bearer` prefix followed by a space is
also accepted by agentgateway). For example, read the value from a protected
local file; do not put the value in command history or blueprint variables:

```sh
kubectl -n agentgateway-system create secret generic openai-credentials \
  --from-file=Authorization=/secure/path/openai-key
```

Repeat your original `gcluster create` command with all its configuration
overrides, adding `--overwrite-deployment` and
`--vars openai_secret_name=openai-credentials`. Preserve the identity mode, both
service-account emails, project, deployment name, runner CIDR, region, namespace,
and model settings. For Anthropic, use `anthropic_secret_name`. Then deploy the
updated configuration.

| Provider | Endpoint path | Model selection |
| --- | --- | --- |
| Gemini Enterprise Agent Platform | `/v1/chat/completions` | Blueprint `vertex_model` |
| OpenAI | `/openai/v1/chat/completions` | Request `model` |
| Anthropic | `/anthropic/v1/chat/completions` | Request `model` |

No provider secret is created or read by Terraform. Only secret names enter its
state. Deployment readiness checks whether the controller accepts the backend
configuration and its Secret reference. Send a request to verify that the provider
accepts the credentials. Secrets in this dedicated cluster are lost when the
cluster is destroyed; keep the original credentials in your existing secret system.

### Test external providers in a Kubernetes Job

After deploying the provider configuration, run these commands from the repository
root on your local machine. The helper creates a temporary Job inside the cluster
to test synchronous and streaming responses through the internal load balancer.
Your machine needs Kubernetes API access, but no direct VPC connection. See
[Run the smoke test in a Kubernetes Job](#run-the-smoke-test-in-a-kubernetes-job)
for access requirements.

Replace the model placeholders with IDs available to your provider accounts and
use your configured namespace if it differs from the default:

```sh
python3 tools/cloud-build/daily-tests/ansible_playbooks/test-validation/agentgateway/run-in-cluster.py \
  --namespace agentgateway-system \
  --provider openai \
  --model OPENAI_MODEL_ID

python3 tools/cloud-build/daily-tests/ansible_playbooks/test-validation/agentgateway/run-in-cluster.py \
  --namespace agentgateway-system \
  --provider anthropic \
  --model ANTHROPIC_MODEL_ID
```

The gateway uses the configured provider Secrets; do not pass API keys to the
helper. These tests make real provider requests. The helper also checks proxy
metrics and removes its temporary resources when finished, including on failure.
An HTTP 403 still requires investigating gateway or provider access; running the
client inside the cluster does not bypass those checks.

### Select a model and token budget

The client options are defined in `smoke-test.py` and shared by the Kubernetes
runner. Both commands accept `--max-output-tokens`
(default: `128`) and `--token-parameter` (default: `max_tokens`). For a model that
requires `max_completion_tokens`, for example, run:

```sh
python3 tools/cloud-build/daily-tests/ansible_playbooks/test-validation/agentgateway/run-in-cluster.py \
  --provider openai --model OPENAI_MODEL_ID \
  --token-parameter max_completion_tokens --max-output-tokens 2048
```

The budget includes reasoning tokens where the model counts them. Increase it if
the model exhausts its budget before producing visible output. The test requires
nonempty synchronous and streaming output and does not fall back to another model.
The existing thinking-budget override applies only to `gemini-2.5-flash` models.

The following models passed synchronous, streaming and metrics checks through
the internal load balancer on September 15, 2026, using the default token settings:

| Provider | Tested model |
| --- | --- |
| Gemini | `gemini-2.5-flash` |
| OpenAI | `gpt-3.5-turbo` |
| Anthropic | `claude-haiku-4-5-20251001` |

Other models must support the gateway's OpenAI-compatible request format and be
available to your provider account. These results do not establish compatibility
with every model from each provider.

## Logging, metrics and troubleshooting

The GKE module enables workload logging and Managed Prometheus. A `PodMonitoring`
resource scrapes proxy metrics on port 15020. Load-balancer request logging is
also enabled. No prompt-body or credential logging is configured.

```sh
kubectl -n agentgateway-system get gateways,httproutes,agentgatewaybackends
kubectl -n agentgateway-system describe gateway agentgateway-internal
kubectl -n agentgateway-system get jobs,pods
kubectl -n agentgateway-system logs deployment/agentgateway-proxy
```

Inspect failed readiness Jobs before retrying; their logs are retained on failure
and replaced on the next Helm upgrade. A Gateway API version error means the
GKE-managed bundle is unsupported or unrecognized. For load-balancer health,
check `HealthCheckPolicy`, the proxy readiness port 15021, NEGs and the firewall
rule allowing the proxy-only subnet and Google health-check ranges. For model inference
403 errors, check the proxy service-account annotation, workload-identity binding,
prediction role, enabled API and model availability. A deployed gateway can still
fail prediction requests because of insufficient model quota.

## Cleanup

```sh
./gcluster destroy agw-demo
```

If cleanup fails, inspect the error and retry the destroy command; do not manually
delete the cluster first. A separately configured Terraform state bucket is not
removed by this blueprint.

For maintainer checks and integration testing, see [TESTING.md](TESTING.md).
