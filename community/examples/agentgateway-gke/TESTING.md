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
used by static validation. The default is v1.7.0; see the live validation results below. When changing it, review the Gateway API readiness check and
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
python -m pip install --require-hashes -r tools/cloud-build/requirements.txt
make agentgateway-static test-agentgateway
```

Run `deactivate` when finished. In subsequent sessions, run
`source venv-agentgateway/bin/activate` from the repository root to reuse the
environment without reinstalling its packages.

Also run repository pre-commit checks and generate/validate all four Terraform
groups. The static script validates agentgateway schemas, optional-provider
rendering and embedded Python syntax. Server-side CRD/CEL validation and live
reconciliation are covered by deployment, not by the static schema check.

### Python helper coverage

To measure unit-test coverage of the readiness and smoke-test helpers, use the
same virtual environment:

```sh
python -m coverage run --data-file=/tmp/agentgateway.coverage \
  --source=community/examples/agentgateway-gke \
  -m unittest discover -s tests -p test_agentgateway.py
python -m coverage report --data-file=/tmp/agentgateway.coverage --fail-under=80
```

This measures the runtime Python helpers. The repository's Go coverage gates
are enforced separately by `make tests` using `tools/enforce_coverage.pl`.

## Deployment readiness

The blueprint's `files/readiness.py` runs in Helm hook Jobs during deployment.
It checks Gateway API compatibility and waits for current Gateway, HTTPRoute and
backend conditions, the controller-created proxy Deployment, and an ingress
address and attached ingress policies. Helm's chart resource waits do not cover all of these resources and
conditions. Network exposure assertions belong to the Kubernetes runner below.

## Validate an existing deployment

Follow [Run the smoke test in a Kubernetes Job](README.md#run-the-smoke-test-in-a-kubernetes-job)
to configure cluster access and test Gemini. The Kubernetes runner
(`run_in_cluster.py`) loads the client argument definitions from `smoke_test.py`
and copies that same file into the Job's ConfigMap. It forwards client options
to the Job; request checks and client defaults are maintained in `smoke_test.py`.
The runner defaults to
`--provider gemini`; use `--provider openai` or `--provider anthropic` with a model
ID for that provider to test existing external-provider routes. The runner selects
Anthropic Messages for Anthropic and OpenAI Chat Completions for other providers. See
[Test external providers](README.md#test-external-providers-in-a-kubernetes-job)
for complete commands. External-provider selection cannot be combined with
`--mock-provider`.

To test a synthetic provider without
making real model requests, run from the repository root:

```sh
python3 community/examples/agentgateway-gke/run_in_cluster.py \
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
the Job reuses the example's `smoke_test.py` client. Its init container first
waits up to 120 seconds for the synthetic Service to accept HTTP connections;
Deployment rollout alone does not establish Service reachability. Failures print
pod, endpoint and backend diagnostics before temporary resources are removed.
Synthetic backend cleanup uses foreground deletion with a 120-second timeout
and waits for the server pods before the runner returns successfully. Client Job
and ConfigMap deletion is asynchronous. Cleanup failures cause a failed run
without hiding an earlier smoke-test failure.
The client retries connection failures and HTTP 502/503/504 responses for up to
five minutes while load-balancer backend health propagates. Authentication errors
fail immediately.

## Testing configuration updates

Test a fresh deployment and an update that changes `vertex_model` or a provider
Secret name. Each configuration release contains its readiness hook, so Helm
must run the hook after applying the updated resources. A missing provider Secret
must prevent readiness; create the Secret and retry to verify recovery. Confirm
that an unchanged reapplication produces no changes with Google provider 8.x.
An older deployment's lock file can retain provider 7.x; upgrading dependencies
requires separate validation.

These changes consolidate Helm releases and remove conditional IAM resource
addresses from the unpublished example. Recreate disposable deployments made
from earlier PR revisions. Do not apply this revision over an existing deployment
without planning the Helm ownership and Terraform state migration.

## Recorded provider validation

On October 28, 2026, agentgateway v1.7.0 passed the full Linux `make tests`
suite, pinned-chart static validation (including optional providers), all 59
Python helper tests and applicable pre-commit checks. The published compatibility
matrix retains Gateway API 1.4–1.6 support, so the readiness version check is
unchanged.

A live v1.6.0-to-v1.7.0 upgrade updated both Helm releases and rolled out the
v1.7.0 controller and proxy images. It used administrator-provisioned identities,
GKE `1.35.8-gke.1919000`, Gateway API v1.5.0 and Google provider 8.6.0.
Gemini `gemini-2.5-flash` and the synthetic provider passed synchronous, streaming
and metrics checks through the internal load balancer. Synthetic resources and
server pods were absent after the runner exited. A missing provider Secret
blocked readiness with `PartiallyValid`; adding a dummy Secret allowed the
configuration update to complete. After restoring the base configuration,
unchanged reapplication reported all four groups already applied.

On October 28, 2026, the review follow-ups passed Linux
`make test-agentgateway agentgateway-static`, applicable pre-commit checks and
59 Python helper tests with 93% runtime-helper coverage. A fresh deployment using
the October 26 versions below passed Gemini and synthetic synchronous, streaming
and metrics checks. The synthetic runner left no backend resources or server
pods after exit. A missing provider Secret blocked readiness with a
`PartiallyValid` diagnostic; adding a dummy Secret allowed the upgrade to finish.
Restoring the base configuration and reapplying required no infrastructure changes.

On October 26, 2026, the full Linux `make tests` suite and the standard
Terraform-console/Helm static checks passed. The 55 agentgateway helper tests
provide 93% runtime-helper coverage. Live testing found and fixed a nested
`templatefile` call rejected by Terraform and an overly short retry window during
initial load-balancer backend propagation.

The fresh deployment used administrator-provisioned identities, agentgateway
v1.6.0, GKE `1.35.8-gke.1919000`, Gateway API v1.5.0 and Google provider 8.6.0.
Gemini `gemini-2.5-flash` and the synthetic provider passed synchronous, streaming
and metrics checks through the internal load balancer. A model change reran the
gateway readiness hook. The alternate model `gemini-2.5-flash-lite` returned a
provider 404 (unavailable or inaccessible in the test project), so request tests
use `gemini-2.5-flash`.

A missing provider Secret blocked the upgrade with a `PartiallyValid` diagnostic;
adding a dummy Secret and retrying passed. The negative test shortened the hook
Job's deadline after observing rejection to exercise Helm rollback. It made no
external-provider requests. An ingress backend-policy change also reran its
readiness hook successfully. After restoring the original configuration,
`gcluster deploy` reported all four groups already applied.

On October 20, 2026, a fresh deployment with administrator-provisioned identities
passed using agentgateway v1.6.0, GKE `1.35.8-gke.1626001` and Gateway API v1.5.0.
Gemini `gemini-2.5-flash` and the synthetic provider passed synchronous, streaming
and metrics checks through the internal load balancer. The synthetic test also
verified missing and valid Secret references. Proxy logs appeared in Cloud Logging,
and a second `gcluster deploy` reported all four groups already applied.

These October runs included the shared firewall planning fix in
[PR #6536](https://github.com/GoogleCloudPlatform/cluster-toolkit/pull/6536).
These runs did not test managed IAM provisioning or live OpenAI and Anthropic requests;
credentials for those providers were unavailable.

The following models passed synchronous, streaming and metrics checks through
the internal load balancer on September 15, 2026, using the default token settings
and OpenAI-compatible endpoints. These results predate the change to Anthropic's
native Messages endpoint and do not validate that change:

| Provider | Tested model |
| --- | --- |
| Gemini | `gemini-2.5-flash` |
| OpenAI | `gpt-3.5-turbo` |
| Anthropic | `claude-haiku-4-5-20251001` |

These results record a completed smoke-test run, not a supported-model list.
They do not establish compatibility with every model from each provider.
The smoke client's thinking-budget override applies only to `gemini-2.5-flash`
models.

## Integration testing

### Run the integration playbook

This playbook is **manual-only**: no upstream daily-test build currently schedules
it. Maintainers will configure scheduling, test-project quota and model access
separately. Automated test code is provided, but scheduled CI coverage is not.

Manual runs use local Terraform state by default. Keep the deployment directory
until cleanup succeeds. For Cloud Build, set `deployment_state_bucket` to the
runner's state bucket (the upstream fleet uses `daily-tests-tf-state`). The shared
creation tasks configure remote state and upload the deployment archive so a
terminated build can be recovered. Cloud Build runs must supply `full_build_id`
and a nonempty state bucket; triage uses the shared runner configuration.

The playbook requires Ansible, which `gcluster` does not install. Install it in
the virtual environment used above:

```sh
source venv-agentgateway/bin/activate
python -m pip install --require-hashes -r tools/cloud-build/requirements.txt
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
  -e "blueprint_yaml=$PWD/community/examples/agentgateway-gke/agentgateway-existing.yaml" \
  -e "node_service_account_email=nodes@YOUR_PROJECT.iam.gserviceaccount.com proxy_service_account_email=proxy@YOUR_PROJECT.iam.gserviceaccount.com"
```

Both configurations run the same Gemini and synthetic-provider request checks. Existing
mode verifies that supplied service accounts still exist after cleanup; managed
mode verifies that the created accounts are gone. Run managed mode separately to
cover IAM provisioning. The `create_service_accounts` playbook variable selects cleanup expectations;
it does not modify the blueprint. Use the edited copy described in the README
and confirm its generated Terraform plan contains no IAM resources.

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
by each model. Anthropic requires `max_tokens`. Omitting token settings uses `max_tokens: 128`, matching the smoke
client defaults. Credentials are provisioned only by your setup tasks, not by the
smoke-test scripts. For an existing cluster, use the
[provider smoke-test commands](README.md#test-external-providers-in-a-kubernetes-job)
instead of the deployment-and-destroy playbook.
