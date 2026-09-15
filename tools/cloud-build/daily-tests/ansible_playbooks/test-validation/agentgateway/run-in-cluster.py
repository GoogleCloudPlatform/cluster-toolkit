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

"""Run a disposable in-cluster smoke client against the internal load balancer."""

import argparse
import importlib.util
import ipaddress
import json
from pathlib import Path
import subprocess
import time
import sys
import uuid


SMOKE_PATH = Path(__file__).resolve().parents[6] / "community/examples/agentgateway-gke/smoke-test.py"
spec = importlib.util.spec_from_file_location("agentgateway_smoke_client", SMOKE_PATH)
smoke_client = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke_client)


def parse_args(argv=None):
    # Separate runner options from client options so new client flags are forwarded
    # without maintaining a second list of names or defaults here.
    runner = argparse.ArgumentParser(add_help=False, allow_abbrev=False)
    runner.add_argument("--namespace", default="agentgateway-system")
    runner.add_argument("--provider", choices=("gemini", "openai", "anthropic"),
                        default="gemini", help="Provider route to test (default: gemini)")
    runner.add_argument("--mock-provider", action="store_true",
                        help="Test a synthetic backend; --model defaults to synthetic")
    parser = argparse.ArgumentParser(description=__doc__, parents=[runner], allow_abbrev=False)
    smoke_client.add_client_arguments(parser)
    args = parser.parse_args(argv)
    if args.mock_provider and args.provider != "gemini":
        parser.error("--mock-provider cannot be combined with an external --provider")
    if args.mock_provider and args.model is None:
        args.model = "synthetic"
    smoke_client.validate_client_arguments(parser, args)
    _, args.client_arguments = runner.parse_known_args(argv)
    if args.mock_provider:
        # Explicit user input follows this default and takes precedence.
        args.client_arguments = ["--model", "synthetic"] + args.client_arguments
    return args


def main():
    args = parse_args()

    def kubectl(*arguments, document=None):
        return subprocess.check_output(
            ["kubectl", "-n", args.namespace, *arguments],
            input=json.dumps(document).encode() if document is not None else None,
            timeout=60,
        ).decode()

    def get(kind, name):
        return json.loads(kubectl("get", kind, name, "-o", "json"))

    gateway = get("gateway", "agentgateway-internal")
    address = gateway["status"]["addresses"][0]["value"]
    if not ipaddress.ip_address(address).is_private:
        raise ValueError("Expected an internal load-balancer address")
    services = json.loads(kubectl("get", "services", "-o", "json"))["items"]
    if any(s["spec"]["type"] == "LoadBalancer" or s["spec"].get("externalIPs") for s in services):
        raise ValueError("Unexpected externally exposed Service")
    if get("service", "agentgateway-proxy")["spec"]["type"] != "ClusterIP":
        raise ValueError("Expected a ClusterIP proxy Service")
    pods = json.loads(kubectl("get", "pods", "-l", "gateway.networking.k8s.io/gateway-name=agentgateway-proxy", "-o", "json"))["items"]
    pod = next(p for p in pods if p["status"].get("phase") == "Running")
    if pod["spec"]["serviceAccountName"] != "agentgateway-proxy":
        raise ValueError("Proxy is using an unexpected Kubernetes service account")
    sa = get("serviceaccount", "agentgateway-proxy")
    if not sa["metadata"].get("annotations", {}).get("iam.gke.io/gcp-service-account"):
        raise ValueError("Missing Workload Identity annotation")
    client = "agentgateway-smoke-" + uuid.uuid4().hex[:8]
    mock_resources = []
    endpoint = "http://" + address
    if args.provider != "gemini":
        endpoint += "/" + args.provider
    if args.mock_provider:
        metadata = {"name": client}
        mock_resources = [
            {"apiVersion": "v1", "kind": "ConfigMap", "metadata": metadata,
             "data": {"server.py": Path(__file__).with_name("mock-provider.py").read_text()}},
            {"apiVersion": "v1", "kind": "Service", "metadata": metadata,
             "spec": {"selector": {"app": client}, "ports": [{"port": 8080, "targetPort": 8080}]}},
            {"apiVersion": "apps/v1", "kind": "Deployment", "metadata": metadata,
             "spec": {"replicas": 1, "selector": {"matchLabels": {"app": client}},
                      "template": {"metadata": {"labels": {"app": client}}, "spec": {
                          "automountServiceAccountToken": False,
                          "containers": [{"name": "mock", "image": "python:3.12.12-slim",
                              "command": ["python3", "/scripts/server.py"],
                              "readinessProbe": {"httpGet": {"path": "/", "port": 8080}},
                              "resources": {"requests": {"cpu": "50m", "memory": "64Mi"},
                                            "limits": {"memory": "128Mi"}},
                              "volumeMounts": [{"name": "scripts", "mountPath": "/scripts", "readOnly": True}]}],
                          "volumes": [{"name": "scripts", "configMap": {"name": client}}],
                      }}}},
            {"apiVersion": "agentgateway.dev/v1alpha1", "kind": "AgentgatewayBackend", "metadata": metadata,
             "spec": {"ai": {"provider": {"openai": {}, "host": f"{client}.{args.namespace}.svc.cluster.local", "port": 8080}},
                      "policies": {"auth": {"secretRef": {"name": client}}}}},
            {"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute", "metadata": metadata,
             "spec": {"parentRefs": [{"name": "agentgateway-proxy"}], "rules": [{
                 "matches": [{"path": {"type": "PathPrefix", "value": "/" + client}}],
                 "filters": [{"type": "URLRewrite", "urlRewrite": {"path": {
                     "type": "ReplacePrefixMatch", "replacePrefixMatch": "/"}}}],
                 "backendRefs": [{"group": "agentgateway.dev", "kind": "AgentgatewayBackend", "name": client}],
             }]}},
        ]
        endpoint += "/" + client

    def wait_backend(accepted):
        deadline = time.monotonic() + 120
        while time.monotonic() < deadline:
            conditions = get("agentgatewaybackend", client).get("status", {}).get("conditions", [])
            if any(c["type"] == "Accepted" and c["status"] == accepted for c in conditions):
                return
            time.sleep(2)
        raise TimeoutError(f"Mock backend did not become Accepted={accepted}")

    config = {
        "apiVersion": "v1", "kind": "ConfigMap",
        "metadata": {"name": client + "-client"},
        "data": {"smoke-test.py": SMOKE_PATH.read_text()},
    }
    job = {
        "apiVersion": "batch/v1", "kind": "Job", "metadata": {"name": client},
        "spec": {
            "backoffLimit": 0, "activeDeadlineSeconds": 600,
            "template": {"spec": {
                "restartPolicy": "Never", "automountServiceAccountToken": False,
                "containers": [{
                    "name": "smoke", "image": "python:3.12.12-slim",
                    "command": ["python3", "-u", "/scripts/smoke-test.py"],
                    "args": ["--endpoint", endpoint,
                             "--metrics-endpoint", f"http://{pod['status']['podIP']}:15020/metrics"] + args.client_arguments,
                    "volumeMounts": [{"name": "scripts", "mountPath": "/scripts", "readOnly": True}],
                    "resources": {"requests": {"cpu": "50m", "memory": "64Mi"},
                                  "limits": {"memory": "128Mi"}},
                }],
                "volumes": [{"name": "scripts", "configMap": {"name": client + "-client"}}],
            }},
        },
    }
    if args.mock_provider:
        # Rollout readiness does not guarantee Service forwarding is ready.
        # Check the Service from the client before testing the gateway path.
        config["data"]["wait-for-http.py"] = Path(__file__).with_name("wait-for-http.py").read_text()
        job["spec"]["template"]["spec"]["initContainers"] = [{
            "name": "wait-for-provider", "image": "python:3.12.12-slim",
            "command": ["python3", "-u", "/scripts/wait-for-http.py"],
            "args": [f"http://{client}.{args.namespace}.svc.cluster.local:8080/"],
            "volumeMounts": [{"name": "scripts", "mountPath": "/scripts", "readOnly": True}],
            "resources": {"requests": {"cpu": "50m", "memory": "64Mi"},
                          "limits": {"memory": "128Mi"}},
        }]
    try:
        if args.mock_provider:
            kubectl("create", "-f", "-", document={"apiVersion": "v1", "kind": "List", "items": mock_resources})
            wait_backend("False")
            secret = {"apiVersion": "v1", "kind": "Secret", "metadata": {"name": client},
                      "type": "Opaque", "stringData": {"Authorization": "synthetic-test-key"}}
            mock_resources.append(secret)
            kubectl("create", "-f", "-", document=secret)
            wait_backend("True")
            kubectl("rollout", "status", "deployment/" + client, "--timeout=45s")
            deadline = time.monotonic() + 120
            while time.monotonic() < deadline:
                route = get("httproute", client)
                parents = route.get("status", {}).get("parents", [])
                if any(all(any(c["type"] == kind and c["status"] == "True"
                                   and c.get("observedGeneration", 0) >= route["metadata"]["generation"]
                                   for c in parent.get("conditions", []))
                               for kind in ("Accepted", "ResolvedRefs")) for parent in parents):
                    break
                time.sleep(2)
            else:
                raise TimeoutError("Synthetic route did not become ready")
        kubectl("create", "-f", "-", document=config)
        kubectl("create", "-f", "-", document=job)
        deadline = time.monotonic() + 660
        while time.monotonic() < deadline:
            status = get("job", client).get("status", {})
            if status.get("succeeded"):
                print(kubectl("logs", "job/" + client), end="")
                return
            if status.get("failed"):
                print(kubectl("logs", "job/" + client), end="")
                raise RuntimeError("Smoke client failed")
            time.sleep(5)
        raise TimeoutError("Smoke client timed out")
    except Exception:
        # Preserve evidence before the finally block removes temporary resources.
        diagnostics = [("get", "pods", "-o", "wide"),
                       ("logs", "job/" + client, "--all-containers", "--tail=50")]
        if args.mock_provider:
            diagnostics += [
                ("get", "endpointslices", "-l", "kubernetes.io/service-name=" + client, "-o", "json"),
                ("describe", "deployment/" + client),
                ("logs", "deployment/" + client, "--tail=50"),
            ]
        for arguments in diagnostics:
            print("Diagnostics: kubectl " + " ".join(arguments), file=sys.stderr)
            try:
                print(kubectl(*arguments), file=sys.stderr)
            except (subprocess.CalledProcessError, subprocess.TimeoutExpired) as error:
                print(f"Diagnostic unavailable: {error}", file=sys.stderr)
                if error.output:
                    print(error.output.decode(), file=sys.stderr)
        raise
    finally:
        kubectl("delete", "job", client, "--ignore-not-found", "--wait=true")
        kubectl("delete", "configmap", client + "-client", "--ignore-not-found")
        if mock_resources:
            kubectl("delete", "-f", "-", "--ignore-not-found", document={
                "apiVersion": "v1", "kind": "List", "items": list(reversed(mock_resources))})


if __name__ == "__main__":
    main()
