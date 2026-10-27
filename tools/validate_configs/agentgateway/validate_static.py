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

import yaml


ROOT = Path(__file__).resolve().parents[3] / "community/examples/agentgateway-gke"


def render_text(name, variables, cwd):
    expression = f'templatefile({json.dumps(str(ROOT / "files" / name))}, jsondecode({json.dumps(json.dumps(variables))}))'
    output = subprocess.check_output(["terraform", "console"], input=("jsonencode(" + expression + ")\n").encode(), cwd=cwd)
    return json.loads(json.loads(output))


def render(name, variables, cwd):
    return [resource for resource in yaml.safe_load_all(render_text(name, variables, cwd)) if resource]


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


def require(valid, message):
    if not valid:
        raise ValueError(message)


def require_schemas(schemas):
    required = {("agentgateway.dev/v1alpha1", kind)
                for kind in ("AgentgatewayBackend", "AgentgatewayParameters")}
    missing = required - schemas.keys()
    require(not missing, f"Chart render is missing required CRD schemas: {sorted(missing)}")


def validate_ingress(resources):
    expected = {"Gateway", "HTTPRoute", "HealthCheckPolicy", "GCPBackendPolicy"}
    objects = {r["kind"]: r for r in resources if r["kind"] in expected}
    require(set(objects) == expected, "Ingress render is missing expected resources")
    require(sum(r["kind"] in expected for r in resources) == len(expected), "Duplicate ingress resources")
    require(objects["Gateway"]["spec"]["gatewayClassName"] == "gke-l7-rilb", "Expected internal GatewayClass")
    route = objects["HTTPRoute"]["spec"]
    require(route["parentRefs"][0]["name"] == objects["Gateway"]["metadata"]["name"], "Incorrect ingress parent")
    require(route["rules"][0]["backendRefs"] == [{"name": "agentgateway-proxy", "port": 8080}], "Incorrect proxy backend")
    for kind in ("HealthCheckPolicy", "GCPBackendPolicy"):
        require(objects[kind]["spec"]["targetRef"] == {
            "group": "", "kind": "Service", "name": "agentgateway-proxy"}, f"Incorrect {kind} target")
    health = objects["HealthCheckPolicy"]["spec"]["default"]["config"]["httpHealthCheck"]
    require(health["port"] == 15021 and health["requestPath"] == "/healthz/ready", "Incorrect health check")


def validate_hook(resources):
    jobs = [r for r in resources if r["kind"] == "Job"]
    require(len(jobs) == 1, "Each stage must contain exactly one readiness Job")
    job = jobs[0]
    require(job["metadata"]["annotations"]["helm.sh/hook"] == "post-install,post-upgrade", "Missing upgrade hook")
    require(job["spec"]["activeDeadlineSeconds"] == 900 and job["spec"]["backoffLimit"] == 2, "Incorrect hook retry bounds")
    compile(job["spec"]["template"]["spec"]["containers"][0]["args"][0], "readiness", "exec")


def main():
    blueprint = yaml.safe_load((ROOT / "agentgateway-gke.yaml").read_text())
    agentgateway_version = blueprint["vars"]["agentgateway_version"]
    with tempfile.TemporaryDirectory() as directory:
        schemas = {}
        for chart in ("agentgateway-crds", "agentgateway"):
            rendered = subprocess.check_output([
                "helm", "template", chart, f"oci://cr.agentgateway.dev/charts/{chart}",
                "--version", agentgateway_version, "--namespace", "agentgateway-system", "--include-crds",
                "--set", "controller.extraEnv.AGW_ENABLE_EXPERIMENTAL_GATEWAY_API_FEATURES=false",
            ])
            for resource in yaml.safe_load_all(rendered):
                if resource and resource.get("kind") == "CustomResourceDefinition":
                    for version in resource["spec"]["versions"]:
                        schema = version["schema"]["openAPIV3Schema"]
                        strict(schema)
                        schemas[(resource["spec"]["group"] + "/" + version["name"],
                                 resource["spec"]["names"]["kind"])] = schema
        require_schemas(schemas)
        import jsonschema

        common = {"namespace": "agentgateway-system",
                  "script": (ROOT / "files/readiness.py").read_text()}
        gateway_hook = render_text("readiness.yaml.tftpl", dict(common, stage="gateway"), directory)
        for optional in (False, True):
            resources = render("gateway.yaml.tftpl", dict(common, readiness_manifest=gateway_hook,
                service_account="proxy@test-project.iam.gserviceaccount.com",
                project_id="test-project", vertex_location="global", vertex_model="gemini-2.5-flash",
                openai_secret_name="existing-openai" if optional else "",
                anthropic_secret_name="existing-anthropic" if optional else "",
            ), directory)
            require(sum(r["kind"] == "AgentgatewayBackend" for r in resources) == (3 if optional else 1), "Invalid rendered gateway configuration")
            require(not any(r["kind"] == "Secret" for r in resources), "Invalid rendered gateway configuration")
            routes = {r["metadata"]["name"]: r["spec"]["rules"][0]
                      for r in resources if r["kind"] == "HTTPRoute"}
            expected = {"vertex-ai": "/v1/chat/completions"}
            if optional:
                expected.update(openai="/openai/v1/chat/completions", anthropic="/v1/messages")
            require(set(routes) == set(expected), "Invalid rendered gateway configuration")
            for name, path in expected.items():
                require(routes[name]["matches"] == [{"path": {"type": "Exact", "value": path}}], "Invalid rendered gateway configuration")
                if name == "openai":
                    require(routes[name]["filters"] == [{"type": "URLRewrite", "urlRewrite": {
                        "path": {"type": "ReplaceFullPath", "replaceFullPath": "/v1/chat/completions"}}}], "Invalid rendered gateway configuration")
                else:
                    require("filters" not in routes[name], "Invalid rendered gateway configuration")
            validate_hook(resources)
            for resource in resources:
                schema = schemas.get((resource["apiVersion"], resource["kind"]))
                if resource["apiVersion"].startswith("agentgateway.dev/"):
                    require(schema is not None, f"Missing schema for {resource['kind']}")
                if schema is not None:
                    jsonschema.validate(resource, schema)
        ingress_hook = render_text("readiness.yaml.tftpl", dict(common, stage="ingress"), directory)
        ingress = render("ingress.yaml.tftpl", dict(common, readiness_manifest=ingress_hook), directory)
        validate_ingress(ingress)
        validate_hook(ingress)
        for stage in ("gateway-api", "gateway", "ingress"):
            resources = render("readiness.yaml.tftpl", dict(common, stage=stage,
                script=(ROOT / "files" / "readiness.py").read_text()), directory)
            validate_hook(resources)
    print("Pinned charts and blueprint templates validated (including optional providers)")


if __name__ == "__main__":
    main()
