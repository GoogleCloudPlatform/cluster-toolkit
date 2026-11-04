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

"""Bounded, read-only Kubernetes readiness checks run by Helm hook Jobs."""

import json
import os
import re
from pathlib import Path
import ssl
import time
import urllib.error
import urllib.request


def current_condition(obj, name):
    """Return a condition only when it reflects the current generation."""
    generation = obj.get("metadata", {}).get("generation", 0)
    return next((c for c in (obj.get("status") or {}).get("conditions") or []
                 if c.get("type") == name
                 and c.get("observedGeneration", generation) >= generation), {})


def condition(obj, name, expected="True"):
    return current_condition(obj, name).get("status") == expected


def backend_auth_status(backend, missing_secret=None):
    """Return readiness and a diagnostic; optionally expect a missing Secret."""
    accepted = current_condition(backend, "Accepted")
    partial = accepted.get("reason") == "PartiallyValid"
    message = accepted.get("message", "Partially valid backend" if partial else "")
    if missing_secret is not None:
        # v1.6 accepts partially valid backends while reporting missing auth.
        ready = ((accepted.get("status") == "False" and not partial) or
                 (accepted.get("status") == "True" and partial
                  and f"secret {missing_secret} not found" in message))
    else:
        ready = accepted.get("status") == "True" and not partial
    return ready, message


def route_ready(route, gateway_name):
    """Require current successful conditions for the specified Gateway parent."""
    namespace = route.get("metadata", {}).get("namespace")
    parents = [p for p in (route.get("status") or {}).get("parents") or []
               if (p.get("parentRef") or {}).get("name") == gateway_name
               and (p.get("parentRef") or {}).get("namespace", namespace) == namespace]
    return bool(parents) and all(
        all(condition({"metadata": route.get("metadata", {}), "status": parent}, name)
            for name in ("Accepted", "ResolvedRefs")) for parent in parents)


def ready(get, namespace, stage):
    base = f"/apis/gateway.networking.k8s.io/v1/namespaces/{namespace}"
    if stage == "gateway-api":
        for name in ("gateways", "httproutes", "gatewayclasses"):
            crd = get(
                "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/"
                f"{name}.gateway.networking.k8s.io"
            )
            if not condition(crd, "Established"):
                return False
            version = crd["metadata"].get("annotations", {}).get(
                "gateway.networking.k8s.io/bundle-version", ""
            )
            if not re.fullmatch(r"v?1\.[456]\.\d+", version):
                raise ValueError(
                    f"Unsupported or unknown Gateway API bundle: {version!r}; "
                    "this blueprint requires 1.4–1.6. Check agentgateway compatibility: "
                    "https://agentgateway.dev/docs/kubernetes/latest/release-notes/versions/"
                )
        return condition(get("/apis/gateway.networking.k8s.io/v1/gatewayclasses/gke-l7-rilb"), "Accepted")

    gateway_name = "agentgateway-proxy" if stage == "gateway" else "agentgateway-internal"
    gateway = get(f"{base}/gateways/{gateway_name}")
    if not all(condition(gateway, name) for name in ("Accepted", "Programmed")):
        return False
    routes = get(f"{base}/httproutes")["items"]
    matched = []
    for route in routes:
        if not any(p["name"] == gateway_name for p in route["spec"]["parentRefs"]):
            continue
        if not route_ready(route, gateway_name):
            return False
        matched.append(route["metadata"]["name"])
    required = "vertex-ai" if stage == "gateway" else "agentgateway-ingress"
    if required not in matched:
        return False
    if stage == "gateway":
        backends = get(f"/apis/agentgateway.dev/v1alpha1/namespaces/{namespace}/agentgatewaybackends")["items"]
        for backend in backends:
            accepted, message = backend_auth_status(backend)
            if not accepted:
                if message:
                    print(f"Backend {backend.get('metadata', {}).get('name', '?')}: {message}", flush=True)
                return False
        if not backends:
            return False
        deployment = get(f"/apis/apps/v1/namespaces/{namespace}/deployments/agentgateway-proxy")
        status = deployment.get("status") or {}
        return ((status.get("observedGeneration") or 0) >= deployment["metadata"]["generation"]
                and (status.get("availableReplicas") or 0) >= 1)
    for kind in ("healthcheckpolicies", "gcpbackendpolicies"):
        policy = get(f"/apis/networking.gke.io/v1/namespaces/{namespace}/{kind}/agentgateway-proxy")
        if not condition(policy, "Attached"):
            return False
    return bool((gateway.get("status") or {}).get("addresses"))


def main():
    credentials = Path("/var/run/secrets/kubernetes.io/serviceaccount")
    context = ssl.create_default_context(cafile=str(credentials / "ca.crt"))

    def get(path):
        request = urllib.request.Request(
            "https://kubernetes.default.svc" + path,
            headers={"Authorization": "Bearer " + (credentials / "token").read_text().strip()},
        )
        with urllib.request.urlopen(request, context=context, timeout=10) as response:
            return json.load(response)

    stage = os.environ["STAGE"]
    deadline = time.monotonic() + 840
    while time.monotonic() < deadline:
        try:
            if ready(get, os.environ["NAMESPACE"], stage):
                print(f"{stage}: ready")
                return
        except urllib.error.HTTPError as error:
            try:
                if error.code not in (404, 429, 500, 502, 503, 504):
                    raise
            finally:
                error.close()
        except (urllib.error.URLError, TimeoutError):
            pass
        print(f"Waiting for {stage}", flush=True)
        time.sleep(5)
    raise TimeoutError(f"Timed out waiting for {stage}; inspect Gateway/HTTPRoute conditions and pod logs")


if __name__ == "__main__":
    main()
