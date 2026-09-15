# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Render templates and check agentgateway CRDs without a cluster or credentials.

Requires terraform, helm, PyYAML and jsonschema. Downloads pinned OCI charts.
"""

import json
from pathlib import Path
import subprocess
import tempfile

import jsonschema
import yaml


ROOT = Path(__file__).resolve().parents[3] / "community/examples/agentgateway-gke"


def render(name, variables, cwd):
    expression = f'templatefile({json.dumps(str(ROOT / "files" / name))}, jsondecode({json.dumps(json.dumps(variables))}))'
    output = subprocess.check_output(["terraform", "console"], input=("jsonencode(" + expression + ")\n").encode(), cwd=cwd)
    return list(yaml.safe_load_all(json.loads(json.loads(output))))


def strict(schema):
    """Reject misspelled properties except where Kubernetes preserves arbitrary data."""
    if not isinstance(schema, dict):
        return
    if "properties" in schema and not schema.get("x-kubernetes-preserve-unknown-fields"):
        schema.setdefault("additionalProperties", False)
    for value in schema.values():
        if isinstance(value, dict):
            strict(value)
        elif isinstance(value, list):
            for item in value:
                strict(item)


def main():
    blueprint = yaml.safe_load((ROOT / "agentgateway-gke.yaml").read_text())
    agentgateway_version = blueprint["vars"]["agentgateway_version"]
    with tempfile.TemporaryDirectory() as directory:
        schemas = {}
        for chart in ("agentgateway-crds", "agentgateway"):
            rendered = subprocess.check_output([
                "helm", "template", chart, f"oci://cr.agentgateway.dev/charts/{chart}",
                "--version", agentgateway_version, "--namespace", "agentgateway-system",
                "--set", "controller.extraEnv.AGW_ENABLE_EXPERIMENTAL_GATEWAY_API_FEATURES=false",
            ])
            for resource in yaml.safe_load_all(rendered):
                if resource and resource.get("kind") == "CustomResourceDefinition":
                    for version in resource["spec"]["versions"]:
                        schema = version["schema"]["openAPIV3Schema"]
                        strict(schema)
                        schemas[(resource["spec"]["group"] + "/" + version["name"],
                                 resource["spec"]["names"]["kind"])] = schema
        common = {"namespace": "agentgateway-system"}
        for optional in (False, True):
            resources = render("gateway.yaml.tftpl", dict(common,
                service_account="proxy@test-project.iam.gserviceaccount.com",
                project_id="test-project", vertex_location="global", vertex_model="gemini-2.5-flash",
                openai_secret_name="existing-openai" if optional else "",
                anthropic_secret_name="existing-anthropic" if optional else "",
            ), directory)
            assert sum(r["kind"] == "AgentgatewayBackend" for r in resources) == (3 if optional else 1)
            assert not any(r["kind"] == "Secret" for r in resources)
            routes = {r["metadata"]["name"]: r["spec"]["rules"][0]
                      for r in resources if r["kind"] == "HTTPRoute"}
            expected = {"vertex-ai": "/v1/chat/completions"}
            if optional:
                expected.update(openai="/openai/v1/chat/completions", anthropic="/v1/messages")
            assert set(routes) == set(expected)
            for name, path in expected.items():
                assert routes[name]["matches"] == [{"path": {"type": "Exact", "value": path}}]
                if name == "openai":
                    assert routes[name]["filters"] == [{"type": "URLRewrite", "urlRewrite": {
                        "path": {"type": "ReplaceFullPath", "replaceFullPath": "/v1/chat/completions"}}}]
                else:
                    assert "filters" not in routes[name]
            for resource in resources:
                schema = schemas.get((resource["apiVersion"], resource["kind"]))
                if schema:
                    jsonschema.validate(resource, schema)
        render("ingress.yaml.tftpl", common, directory)
        for stage in ("gateway-api", "gateway", "ingress"):
            resources = render("readiness.yaml.tftpl", dict(common, stage=stage,
                script=(ROOT / "files" / "readiness.py").read_text()), directory)
            job = next(r for r in resources if r["kind"] == "Job")
            compile(job["spec"]["template"]["spec"]["containers"][0]["args"][0], "readiness", "exec")
    print("Pinned charts and blueprint templates validated (including optional providers)")


if __name__ == "__main__":
    main()
