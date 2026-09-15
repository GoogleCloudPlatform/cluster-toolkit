# Testing the agentgateway GKE blueprint

This guide covers maintainer checks for the [agentgateway GKE blueprint](README.md).
For deployment and usage checks, see [Validate and use](README.md#validate-and-use).

Local Terraform validation, chart rendering and schema checks do not establish live
GKE compatibility. A successful integration run is required before treating this example
as validated for a particular GKE patch, Gateway API bundle and Gemini model. Record the
resolved GKE patch, Gateway API bundle, agentgateway version and Gemini model with each
integration run's results.

## Updating agentgateway

Set `agentgateway_version` in the blueprint to update both Helm releases and the version
used by static validation. When changing it, review the Gateway API readiness check and
the compatibility range documented in the README. Static validation only reads the blueprint
default (does not read CLI deployment overrides).

## Local checks

Run these commands from the repository root. The static checks require Terraform,
Helm >= 3.12, and Python >= 3.10.

Create and activate a virtual environment to keep test dependencies separate from
your system or Homebrew Python installation, then run the local checks (no cluster):

```sh
python3 -m venv venv-agentgateway
source venv-agentgateway/bin/activate
python -m pip install PyYAML jsonschema
python tools/validate_configs/agentgateway/validate-static.py
python -m unittest tests.test_agentgateway
terraform -chdir=modules/scheduler/gke-cluster init -backend=false
terraform -chdir=modules/scheduler/gke-cluster test -filter=tests/gateway_api.tftest.hcl
terraform -chdir=community/modules/project/agentgateway-identity init -backend=false
terraform -chdir=community/modules/project/agentgateway-identity test
terraform -chdir=community/modules/project/vertex-ai-prediction init -backend=false
terraform -chdir=community/modules/project/vertex-ai-prediction test
```

Run `deactivate` when finished. In subsequent sessions, run
`source venv-agentgateway/bin/activate` from the repository root to reuse the
environment without reinstalling its packages.

Also run repository pre-commit checks and generate/validate all four Terraform
groups. The static script validates agentgateway schemas, optional-provider
rendering and embedded Python syntax. Server-side CRD/CEL validation and live
reconciliation are covered by deployment, not by the static schema check.

## Deployment readiness

The blueprint's `files/readiness.py` runs in Helm hook Jobs during deployment.
It checks Gateway API compatibility and waits for current Gateway, HTTPRoute and
backend conditions, the controller-created proxy Deployment, and an ingress
address. Helm's chart resource waits do not cover all of these resources and
conditions. Network exposure assertions belong to the Kubernetes runner below.

## Validate an existing deployment

Follow [Run the smoke test in a Kubernetes Job](README.md#run-the-smoke-test-in-a-kubernetes-job)
to configure cluster access and test Gemini. The Kubernetes runner
(`run-in-cluster.py`) loads the client argument definitions from `smoke-test.py`
and copies that same file into the Job's ConfigMap. It forwards client options
to the Job; request checks and client defaults are maintained in `smoke-test.py`.
The runner defaults to
`--provider gemini`; use `--provider openai` or `--provider anthropic` with a model
ID for that provider to test existing external-provider routes. See
[Test external providers](README.md#test-external-providers-in-a-kubernetes-job)
for complete commands. External-provider selection cannot be combined with
`--mock-provider`.

To test a synthetic provider without
making real model requests, run from the repository root:

```sh
python3 tools/cloud-build/daily-tests/ansible_playbooks/test-validation/agentgateway/run-in-cluster.py \
  --namespace agentgateway-system \
  --mock-provider
```

The mock uses the model ID `synthetic` by default and supports OpenAI-compatible
chat completions with synchronous JSON and streaming SSE responses. It does not
change behavior based on the model ID or simulate other API formats. Real-provider
tests require `--model`.

Use your configured namespace if it differs from the default. This test checks
missing and valid Secret references, synchronous and streaming responses, and
proxy metrics. The synthetic server is stored beside the integration helper;
the Job reuses the example's `smoke-test.py` client. Its init container first
waits up to 120 seconds for the synthetic Service to accept HTTP connections;
Deployment rollout alone does not establish Service reachability. Failures print
pod, endpoint and backend diagnostics before temporary resources are removed.

## Integration testing

The playbook requires Ansible, which `gcluster` does not install. Install it in
the virtual environment used above:

```sh
source venv-agentgateway/bin/activate
python -m pip install ansible-core
```

The integration playbook deploys, reapplies, checks Gemini synchronous/streaming
requests, a synthetic OpenAI backend with missing/present secrets, metrics and
Cloud Logging. It also checks that reapplication produces no Terraform changes.

This playbook creates and destroys a test deployment, including when validation
fails. Use a dedicated deployment name and directory. After cleanup, it checks
for remaining cluster, network, forwarding rules and service accounts. From the
repository root:

```sh
ansible-playbook \
  tools/cloud-build/daily-tests/ansible_playbooks/agentgateway-integration-test.yml \
  -e @tools/cloud-build/daily-tests/tests/agentgateway.yml \
  -e "workspace=$PWD project=YOUR_PROJECT deployment_name=agw-ci authorized_cidr=YOUR_IP/32"
```

Maintainers can invoke this playbook from their existing test runner and supply
its authorized egress CIDR. Live OpenAI/Anthropic calls are optional and disabled
by default; see the configuration below.

### Existing identities

Run a separate integration test with administrator-provisioned identities,
following the [setup instructions](README.md#use-existing-service-accounts). Use a new
deployment name rather than changing identity mode on a managed deployment:

```sh
ansible-playbook \
  tools/cloud-build/daily-tests/ansible_playbooks/agentgateway-integration-test.yml \
  -e @tools/cloud-build/daily-tests/tests/agentgateway.yml \
  -e "workspace=$PWD project=YOUR_PROJECT deployment_name=agw-existing authorized_cidr=YOUR_IP/32" \
  -e '{"create_service_accounts": false}' \
  -e "node_service_account_email=nodes@YOUR_PROJECT.iam.gserviceaccount.com proxy_service_account_email=proxy@YOUR_PROJECT.iam.gserviceaccount.com"
```

Both modes run the same Gemini and synthetic-provider request checks. Existing
mode verifies that supplied service accounts still exist after cleanup; managed
mode verifies that the created accounts are gone. Run managed mode separately to
cover IAM provisioning. The Terraform module tests also verify that existing
mode contains no managed IAM resources and rejects invalid inputs.

### Optional external-provider integration checks

To include real OpenAI or Anthropic calls in the full integration run, supply
`external_provider_tests` and an absolute path in `external_provider_setup_tasks`.
The setup file is an Ansible task list maintained by your test runner. It must
create the referenced Opaque Secrets, each with an `Authorization` entry, in
`agentgateway_namespace`. Keep credentials outside the repository and mark tasks
that handle them with `no_log: true`.

The playbook first deploys the Gemini configuration and obtains cluster
credentials, then runs your setup tasks, regenerates the deployment with the
provider Secret names, and deploys the provider routes. It runs reapplication,
empty-plan and request checks before destroying the test deployment. Secrets in
that cluster are deleted with the cluster.

For example, save the following non-secret configuration outside the repository:

```yaml
external_provider_setup_tasks: /absolute/path/provision-provider-secrets.yml
external_provider_tests:
- provider: openai
  model: YOUR_OPENAI_MODEL_ID
  secret_name: openai-secret
  token_parameter: max_completion_tokens
  max_output_tokens: 2048
- provider: anthropic
  model: YOUR_ANTHROPIC_MODEL_ID
  secret_name: anthropic-secret
  token_parameter: max_tokens
  max_output_tokens: 256
```

Add `-e @/absolute/path/provider-tests.yml` to the integration command. Use one
entry per provider; omit a provider to skip it. Select token settings supported
by each model. Omitting token settings uses `max_tokens: 128`, matching the smoke
client defaults. Credentials are provisioned only by your setup tasks, not by the
smoke-test scripts. For an existing cluster, use the
[provider smoke-test commands](README.md#test-external-providers-in-a-kubernetes-job)
instead of the deployment-and-destroy playbook.
