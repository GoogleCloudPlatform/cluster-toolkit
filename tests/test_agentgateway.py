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

"""Failure cases that must not produce a green gateway integration test."""

from email.message import Message
import io
import copy
import json
import sys
from pathlib import Path
import unittest
from unittest import mock
import threading
from http.server import HTTPServer
import urllib.request
import urllib.error


ROOT = Path(__file__).resolve().parents[1] / "community/examples/agentgateway-gke"


sys.path[:0] = [str(ROOT), str(ROOT.parents[2] / "tools/validate_configs/agentgateway")]
import smoke_test as smoke
from files import readiness
import wait_for_http as http_readiness
import run_in_cluster as integration
import mock_provider as provider
import validate_static as static


class ProviderSelectionTests(unittest.TestCase):
    def test_forwarded_defaults_match_direct_client(self):
        args = integration.parse_args(["--model", "test-model"])
        direct = smoke.parse_args(["--endpoint", "http://gateway"] + args.client_arguments)
        self.assertEqual(args.model, direct.model)
        self.assertEqual(args.token_parameter, direct.token_parameter)
        self.assertEqual(args.max_output_tokens, direct.max_output_tokens)

    def test_new_client_option_is_forwarded_without_runner_changes(self):
        original = integration.add_client_arguments

        def add_option(parser, **kwargs):
            original(parser, **kwargs)
            parser.add_argument("--test-client-option")

        with mock.patch.object(integration, "add_client_arguments", side_effect=add_option):
            args = integration.parse_args(["--provider", "openai", "--model=test-model",
                                           "--test-client-option", "value", "--namespace", "test"])
        self.assertEqual(args.client_arguments, ["--model=test-model", "--test-client-option", "value"])

    def test_mock_defaults_to_synthetic_model(self):
        args = integration.parse_args(["--mock-provider"])
        self.assertEqual(args.model, "synthetic")
        self.assertTrue(args.mock_provider)
        client = smoke.parse_args(["--endpoint", "http://gateway"] + args.client_arguments)
        self.assertEqual(client.model, "synthetic")

    def test_mock_preserves_explicit_model_for_compatibility(self):
        args = integration.parse_args(["--mock-provider", "--model", "mock"])
        self.assertEqual(args.model, "mock")
        client = smoke.parse_args(["--endpoint", "http://gateway"] + args.client_arguments)
        self.assertEqual(client.model, "mock")

    def test_empty_model_is_rejected_even_for_mock(self):
        with mock.patch("sys.stderr", io.StringIO()):
            with self.assertRaises(SystemExit):
                integration.parse_args(["--mock-provider", "--model", ""])

    def test_real_providers_require_model(self):
        for provider in ("gemini", "openai", "anthropic"):
            with self.subTest(provider=provider), mock.patch("sys.stderr", io.StringIO()) as stderr:
                with self.assertRaises(SystemExit) as error:
                    integration.parse_args(["--provider", provider])
                self.assertEqual(error.exception.code, 2)
                self.assertIn("--model is required", stderr.getvalue())

    def test_job_uses_selected_provider_and_model(self):
        for provider, suffix in ((None, ""), ("openai", "/openai"), ("anthropic", "")):
            with self.subTest(provider=provider):
                jobs = []
                configs = []

                def kubectl(command, input=None, **kwargs):
                    arguments = command[3:]
                    if arguments[:2] == ["create", "-f"]:
                        document = json.loads(input)
                        if document["kind"] == "Job":
                            jobs.append(document)
                        if document["kind"] == "ConfigMap":
                            configs.append(document)
                        return b"created"
                    if arguments[0] != "get":
                        return b""
                    kind = arguments[1]
                    objects = {
                        "gateway": {"status": {"addresses": [{"value": "10.1.0.5"}]}},
                        "services": {"items": []},
                        "service": {"spec": {"type": "ClusterIP"}},
                        "pods": {"items": [{"spec": {"serviceAccountName": "agentgateway-proxy"},
                                              "status": {"phase": "Running", "podIP": "10.4.0.5",
                        "conditions": [{"type": "Ready", "status": "True"}]}}]},
                        "serviceaccount": {"metadata": {"annotations": {"iam.gke.io/gcp-service-account": "proxy@example.com"}}},
                        "job": {"status": {"succeeded": 1}},
                    }
                    return json.dumps(objects[kind]).encode()

                parameter = "max_tokens" if provider == "anthropic" else "max_completion_tokens"
                argv = ["run_in_cluster.py", "--model", "test-model", "--token-parameter",
                        parameter, "--max-output-tokens", "2048"]
                if provider:
                    argv += ["--provider", provider]
                with mock.patch.object(integration.sys, "argv", argv), \
                        mock.patch.object(integration.subprocess, "check_output", side_effect=kubectl):
                    integration.main()
                self.assertEqual(len(jobs), 1)
                self.assertEqual(configs[0]["data"]["smoke_test.py"], (ROOT / "smoke_test.py").read_text())
                args = jobs[0]["spec"]["template"]["spec"]["containers"][0]["args"]
                client = smoke.parse_args(args)
                self.assertEqual(client.endpoint, "http://10.1.0.5" + suffix)
                self.assertEqual(client.model, "test-model")
                self.assertEqual(client.token_parameter, parameter)
                self.assertEqual(client.api_format, "anthropic" if provider == "anthropic" else "openai")
                self.assertEqual(client.max_output_tokens, 2048)
                self.assertEqual(client.metrics_endpoint, "http://10.4.0.5:15020/metrics")

    def test_mock_rejects_external_provider_before_cluster_access(self):
        for provider in ("openai", "anthropic"):
            with self.subTest(provider=provider), \
                    mock.patch.object(integration.sys, "argv", ["run_in_cluster.py", "--model", "test",
                                                               "--provider", provider, "--mock-provider"]), \
                    mock.patch.object(integration.subprocess, "check_output") as kubectl, \
                    mock.patch.object(integration.sys, "stderr", io.StringIO()):
                with self.assertRaises(SystemExit) as error:
                    integration.main()
                self.assertEqual(error.exception.code, 2)
                kubectl.assert_not_called()


def response(events, content_type="text/event-stream"):
    stream = io.BytesIO("\n\n".join("data: " + (e if isinstance(e, str) else json.dumps(e)) for e in events).encode())
    stream.headers = Message()
    stream.headers["Content-Type"] = content_type
    return stream


CONTENT = {"choices": [{"delta": {"content": "hello"}, "finish_reason": None}]}
FINISH = {"choices": [{"delta": {}, "finish_reason": "stop"}]}


class RequestTests(unittest.TestCase):
    def test_provider_requests_preserve_model_path_and_token_budget(self):
        cases = [("/openai", "test-openai", "max_completion_tokens", 2048),
                 ("", "gemini-2.5-flash", "max_tokens", 128)]
        for path, model, parameter, budget in cases:
            with self.subTest(model=model):
                sync = io.BytesIO(json.dumps({"choices": [{"message": {"content": "one"}}]}).encode())
                stream = response([CONTENT, FINISH, "[DONE]"])
                with mock.patch.object(smoke, "open_response", side_effect=[sync, stream]) as send:
                    smoke.smoke("http://gateway" + path, model, parameter, budget)
                self.assertEqual(send.call_count, 2)
                for call, streaming in zip(send.call_args_list, (False, True)):
                    request = call.args[0]
                    self.assertEqual(request.full_url, "http://gateway" + path + "/v1/chat/completions")
                    body = json.loads(request.data)
                    self.assertEqual(body["model"], model)
                    self.assertEqual(body["stream"], streaming)
                    self.assertEqual(body[parameter], budget)
                    other = "max_tokens" if parameter == "max_completion_tokens" else "max_completion_tokens"
                    self.assertNotIn(other, body)
                    if model == "gemini-2.5-flash":
                        self.assertEqual(body["thinking_config"], {"thinkingBudget": 0})
                    else:
                        self.assertNotIn("thinking_config", body)

    def test_default_token_budget_is_preserved(self):
        sync = io.BytesIO(b'{"choices":[{"message":{"content":"one"}}]}')
        with mock.patch.object(smoke, "open_response", side_effect=[sync, response([CONTENT, FINISH, "[DONE]"])]) as send:
            smoke.smoke("http://gateway", "test-model")
        self.assertEqual(json.loads(send.call_args_list[0].args[0].data)["max_tokens"], 128)

    def test_invalid_token_budget_fails_before_network_access(self):
        for module, arguments in [(smoke, ["--endpoint", "http://gateway"]), (integration, [])]:
            for budget in ("0", "-1"):
                with self.subTest(module=module.__name__, budget=budget), \
                        mock.patch.object(sys, "argv",
                                          ["test", "--model", "test", "--max-output-tokens", budget] + arguments), \
                        mock.patch("sys.stderr", io.StringIO()), \
                        mock.patch.object(smoke, "open_response") as send, \
                        mock.patch.object(integration.subprocess, "check_output") as kubectl:
                    with self.assertRaises(SystemExit) as error:
                        module.main()
                    self.assertEqual(error.exception.code, 2)
                    send.assert_not_called()
                    kubectl.assert_not_called()


class StreamingTests(unittest.TestCase):
    def test_complete_stream(self):
        self.assertEqual(smoke.check_stream(response([CONTENT, CONTENT, FINISH, "[DONE]"])), 2)

    def test_truncated_stream(self):
        with self.assertRaisesRegex(ValueError, "without \\[DONE\\]"):
            smoke.check_stream(response([CONTENT, FINISH]))

    def test_empty_stream(self):
        with self.assertRaisesRegex(ValueError, "without content"):
            smoke.check_stream(response([FINISH, "[DONE]"]))

    def test_provider_error_with_http_200(self):
        with self.assertRaisesRegex(ValueError, "SSE error"):
            smoke.check_stream(response([CONTENT, {"error": {"message": "quota exhausted"}}]))

    def test_non_streaming_response(self):
        with self.assertRaisesRegex(ValueError, "text/event-stream"):
            smoke.check_stream(response([CONTENT, FINISH, "[DONE]"], "application/json"))

    def test_missing_finish_reason(self):
        with self.assertRaisesRegex(ValueError, "finish reason"):
            smoke.check_stream(response([CONTENT, "[DONE]"]))


class ReadinessTests(unittest.TestCase):
    def test_old_gateway_status_does_not_pass(self):
        gateway = {"metadata": {"generation": 2}, "status": {"conditions": [
            {"type": "Programmed", "status": "True", "observedGeneration": 1}]}}
        self.assertFalse(readiness.condition(gateway, "Programmed"))

    def test_condition_false_does_not_pass(self):
        gateway = {"metadata": {"generation": 1}, "status": {"conditions": [
            {"type": "Accepted", "status": "False", "observedGeneration": 1}]}}
        self.assertFalse(readiness.condition(gateway, "Accepted"))

    def test_expected_false_condition_must_be_current(self):
        obj = {"metadata": {"generation": 2}, "status": {"conditions": [
            {"type": "Accepted", "status": "False", "observedGeneration": 1}]}}
        self.assertFalse(readiness.condition(obj, "Accepted", expected="False"))
        obj["status"]["conditions"][0]["observedGeneration"] = 2
        self.assertTrue(readiness.condition(obj, "Accepted", expected="False"))

    def test_unknown_gateway_api_version_fails(self):
        def get(_):
            return {"metadata": {"annotations": {"gateway.networking.k8s.io/bundle-version": "v1.2.0"}},
                    "status": {"conditions": [{"type": "Established", "status": "True"}]}}
        with self.assertRaisesRegex(ValueError, "Unsupported"):
            readiness.ready(get, "test", "gateway-api")

    def test_no_routes_is_not_ready(self):
        def get(path):
            if path.endswith("httproutes"):
                return {"items": []}
            return {"status": {"conditions": [
                {"type": t, "status": "True"} for t in ("Accepted", "Programmed")]}}
        self.assertFalse(readiness.ready(get, "test", "gateway"))


class HttpReadinessTests(unittest.TestCase):
    def test_refused_connection_is_retried(self):
        result = io.BytesIO(b"")
        result.status = 200
        with mock.patch.object(http_readiness.urllib.request, "urlopen", side_effect=[
                urllib.error.URLError(ConnectionRefusedError("not listening")), result]) as open_url:
            http_readiness.wait_for_http("http://mock:8080/", timeout=1, interval=0)
        self.assertEqual(open_url.call_count, 2)

    def test_unavailable_http_is_retried(self):
        result = io.BytesIO(b"")
        result.status = 200
        with mock.patch.object(http_readiness.urllib.request, "urlopen", side_effect=[
                urllib.error.HTTPError("http://mock:8080/", 503, "unavailable", {}, None), result]):
            http_readiness.wait_for_http("http://mock:8080/", timeout=1, interval=0)

    def test_auth_error_is_not_retried(self):
        with mock.patch.object(http_readiness.urllib.request, "urlopen", side_effect=
                               urllib.error.HTTPError("http://mock:8080/", 401, "unauthorized", {}, None)) as open_url:
            with self.assertRaises(urllib.error.HTTPError):
                http_readiness.wait_for_http("http://mock:8080/", timeout=1, interval=0)
        self.assertEqual(open_url.call_count, 1)

    def test_unreachable_service_times_out(self):
        with mock.patch.object(http_readiness.urllib.request, "urlopen",
                               side_effect=urllib.error.URLError("connection refused")):
            with self.assertRaisesRegex(TimeoutError, "connection refused"):
                http_readiness.wait_for_http("http://mock:8080/", timeout=0.01, interval=0.001)


class MockProviderTests(unittest.TestCase):
    def setUp(self):
        self.server = HTTPServer(("127.0.0.1", 0), provider.Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.url = f"http://127.0.0.1:{self.server.server_port}/v1/chat/completions"

    def tearDown(self):
        self.server.shutdown()
        self.thread.join()
        self.server.server_close()

    def request(self, streaming=False, key="synthetic-test-key"):
        return urllib.request.urlopen(urllib.request.Request(
            self.url, data=json.dumps({"stream": streaming}).encode(),
            headers={"Authorization": "Bearer " + key}), timeout=5)

    def test_wrong_credentials_are_rejected(self):
        with self.assertRaises(urllib.error.HTTPError) as error:
            self.request(key="wrong")
        self.assertEqual(error.exception.code, 401)
        error.exception.close()

    def test_synchronous_protocol(self):
        with self.request() as result:
            self.assertEqual(json.load(result)["choices"][0]["message"]["content"], "one two")

    def test_streaming_protocol(self):
        with self.request(streaming=True) as result:
            self.assertEqual(smoke.check_stream(result), 2)


class SmokeFailureTests(unittest.TestCase):
    def test_availability_retry_then_success(self):
        result = io.BytesIO(b"ok")
        errors = [urllib.error.HTTPError("http://gateway", 503, "unavailable", {}, None)
                  for _ in range(7)]
        errors += [urllib.error.URLError("connection refused"), TimeoutError("socket timeout"), result]
        with mock.patch.object(smoke.urllib.request, "urlopen", side_effect=errors) as request, \
                mock.patch.object(smoke.time, "sleep"):
            self.assertIs(smoke.open_response("http://gateway"), result)
        self.assertEqual(request.call_count, 10)

    def test_availability_retry_stops_at_deadline(self):
        body = io.BytesIO(b"unavailable")
        error = urllib.error.HTTPError("http://gateway", 503, "unavailable", {}, body)
        with mock.patch.object(smoke.urllib.request, "urlopen", side_effect=error) as request, \
                mock.patch.object(smoke.time, "monotonic", side_effect=[0, 0, 301]):
            with self.assertRaises(urllib.error.HTTPError):
                smoke.open_response("http://gateway")
        self.assertEqual(request.call_count, 1)
        self.assertTrue(body.closed)

    def test_auth_failure_is_not_retried_or_logged(self):
        error = urllib.error.HTTPError("http://gateway", 403, "forbidden", {}, io.BytesIO(b"sensitive"))
        with mock.patch.object(smoke.urllib.request, "urlopen", side_effect=error) as request:
            with self.assertRaisesRegex(RuntimeError, "Gateway returned HTTP 403"):
                smoke.smoke("http://gateway", "model")
        self.assertEqual(request.call_count, 1)

    def test_empty_synchronous_response_fails(self):
        with mock.patch.object(smoke, "open_response", return_value=io.BytesIO(b'{"choices":[]}')):
            with self.assertRaisesRegex(ValueError, "Missing synchronous completion"):
                smoke.smoke("http://gateway", "model")

    def test_metrics_must_contain_agentgateway_samples(self):
        for metrics, valid in [(b"# TYPE agentgateway_requests counter", True), (b"unrelated", False)]:
            with self.subTest(valid=valid), \
                    mock.patch.object(sys, "argv", ["smoke_test.py", "--endpoint", "http://gateway",
                                                   "--model", "model", "--metrics-endpoint", "http://metrics"]), \
                    mock.patch.object(smoke, "smoke"), \
                    mock.patch.object(smoke.urllib.request, "urlopen", return_value=io.BytesIO(metrics)):
                if valid:
                    smoke.main()
                else:
                    with self.assertRaisesRegex(ValueError, "Prometheus"):
                        smoke.main()


class GatewayReadinessTests(unittest.TestCase):
    @staticmethod
    def conditions(*names):
        return [{"type": name, "status": "True", "observedGeneration": 1} for name in names]

    def objects(self, stage):
        gateway = "agentgateway-proxy" if stage == "gateway" else "agentgateway-internal"
        route = "vertex-ai" if stage == "gateway" else "agentgateway-ingress"
        return {
            "gateways/" + gateway: {"metadata": {"generation": 1}, "status": {
                "conditions": self.conditions("Accepted", "Programmed"), "addresses": [{"value": "10.0.0.1"}]}},
            "httproutes": {"items": [{"metadata": {"name": route, "generation": 1},
                "spec": {"parentRefs": [{"name": gateway}]}, "status": {"parents": [{
                    "parentRef": {"name": gateway}, "conditions": self.conditions("Accepted", "ResolvedRefs")} ]}}]},
            "agentgatewaybackends": {"items": [{"metadata": {"generation": 1},
                "status": {"conditions": self.conditions("Accepted")}}]},
            "healthcheckpolicies/agentgateway-proxy": {"metadata": {"generation": 1},
                "status": {"conditions": self.conditions("Attached")}},
            "gcpbackendpolicies/agentgateway-proxy": {"metadata": {"generation": 1},
                "status": {"conditions": self.conditions("Attached")}},
            "deployments/agentgateway-proxy": {"metadata": {"generation": 1},
                "status": {"observedGeneration": 1, "availableReplicas": 1}},
        }

    def test_null_status_is_not_ready(self):
        for status in (None, {"conditions": None}):
            self.assertFalse(readiness.condition({"status": status}, "Accepted"))
        for resource in ("httproutes", "deployments/agentgateway-proxy"):
            objects = self.objects("gateway")
            obj = objects[resource]["items"][0] if resource == "httproutes" else objects[resource]
            obj["status"] = None
            self.assertFalse(readiness.ready(lambda path: next(value for key, value in objects.items()
                                                              if path.endswith('/' + key)), "test", "gateway"))

    def test_route_readiness_requires_current_matching_parent(self):
        route = self.objects("gateway")["httproutes"]["items"][0]
        parent = route["status"]["parents"][0]
        self.assertTrue(readiness.route_ready(route, "agentgateway-proxy"))
        for failure in ("missing", "other", "stale", "false", "namespace"):
            broken = copy.deepcopy(route)
            target = broken["status"]["parents"][0]
            if failure == "missing":
                target["conditions"] = []
            elif failure == "other":
                target["parentRef"]["name"] = "other-gateway"
            elif failure == "namespace":
                target["parentRef"]["namespace"] = "other-namespace"
            elif failure == "stale":
                target["conditions"][0]["observedGeneration"] = 0
            else:
                target["conditions"][0]["status"] = "False"
                unrelated = copy.deepcopy(parent)
                unrelated["parentRef"]["name"] = "other-gateway"
                broken["status"]["parents"].append(unrelated)
            with self.subTest(failure=failure):
                self.assertFalse(readiness.route_ready(broken, "agentgateway-proxy"))

    def test_ready_gateway_and_ingress(self):
        for stage in ("gateway", "ingress"):
            objects = self.objects(stage)
            with self.subTest(stage=stage):
                self.assertTrue(readiness.ready(lambda path: next(value for key, value in objects.items()
                                                                 if path.endswith('/' + key)), "test", stage))

    def test_incomplete_route_backend_and_deployment_are_not_ready(self):
        for missing in ("parents", "conditions", "backend", "deployment", "replicas"):
            objects = self.objects("gateway")
            route = objects["httproutes"]["items"][0]
            if missing == "parents":
                route["status"]["parents"] = []
            elif missing == "conditions":
                route["status"]["parents"][0]["conditions"] = []
            elif missing == "backend":
                objects["agentgatewaybackends"]["items"] = []
            elif missing == "replicas":
                objects["deployments/agentgateway-proxy"]["status"]["availableReplicas"] = 0
            else:
                objects["deployments/agentgateway-proxy"]["status"]["observedGeneration"] = 0
            with self.subTest(missing=missing):
                self.assertFalse(readiness.ready(lambda path: next(value for key, value in objects.items()
                                                                  if path.endswith('/' + key)), "test", "gateway"))

    def test_gateway_api_requires_established_crds_and_accepted_class(self):
        crd = {"metadata": {"annotations": {"gateway.networking.k8s.io/bundle-version": "v1.5.0"}},
               "status": {"conditions": self.conditions("Established")}}
        gateway_class = {"status": {"conditions": self.conditions("Accepted")}}
        get = lambda path: gateway_class if "/gatewayclasses/" in path else crd
        self.assertTrue(readiness.ready(get, "test", "gateway-api"))
        gateway_class["status"]["conditions"] = []
        self.assertFalse(readiness.ready(get, "test", "gateway-api"))
        gateway_class["status"]["conditions"] = self.conditions("Accepted")
        crd["status"]["conditions"] = []
        self.assertFalse(readiness.ready(get, "test", "gateway-api"))

    def test_ingress_requires_address_and_attached_policies(self):
        for key in ("gateways/agentgateway-internal", "healthcheckpolicies/agentgateway-proxy",
                    "gcpbackendpolicies/agentgateway-proxy"):
            for failure in ("missing", "false", "stale"):
                with self.subTest(resource=key, failure=failure):
                    objects = self.objects("ingress")
                    if key.startswith("gateways/"):
                        objects[key]["status"]["addresses"] = []
                    else:
                        conditions = objects[key]["status"]["conditions"]
                        if failure == "missing":
                            conditions.clear()
                        elif failure == "false":
                            conditions[0]["status"] = "False"
                        else:
                            conditions[0]["observedGeneration"] = 0
                    self.assertFalse(readiness.ready(lambda path: next(v for k, v in objects.items()
                        if path.endswith('/' + k)), "test", "ingress"))

    def test_partial_backend_blocks_deployment_and_reports_message(self):
        objects = self.objects("gateway")
        accepted = objects["agentgatewaybackends"]["items"][0]["status"]["conditions"][0]
        accepted.update(reason="PartiallyValid", message="secret test/missing not found")
        with mock.patch("sys.stdout", io.StringIO()) as output:
            self.assertFalse(readiness.ready(lambda path: next(v for k, v in objects.items()
                if path.endswith('/' + k)), "test", "gateway"))
        self.assertIn("secret test/missing not found", output.getvalue())

    def test_readiness_job_retries_transient_api_failure(self):
        result = io.BytesIO(b'{"status":{"conditions":[]}}')
        error_body = io.BytesIO(b"unavailable")
        def ready(get, namespace, stage):
            get('/test')
            return True
        with mock.patch.object(readiness.ssl, "create_default_context"), \
                mock.patch.object(readiness.Path, "read_text", return_value="test-token"), \
                mock.patch.dict(readiness.os.environ, {"STAGE": "gateway", "NAMESPACE": "test"}), \
                mock.patch.object(readiness, "ready", side_effect=ready), \
                mock.patch.object(readiness.urllib.request, "urlopen", side_effect=[
                    urllib.error.HTTPError("http://api", 503, "unavailable", {}, error_body), result]), \
                mock.patch.object(readiness.time, "sleep"):
            readiness.main()
        self.assertTrue(error_body.closed)


class RunnerLifecycleTests(unittest.TestCase):
    def test_partially_valid_backend_auth_status(self):
        accepted = {"type": "Accepted", "status": "True", "reason": "PartiallyValid",
                    "observedGeneration": 2, "message": "failed to translate backend: secret test/mock not found"}
        backend = {"metadata": {"generation": 2}, "status": {"conditions": [accepted]}}
        self.assertTrue(integration.backend_auth_status(backend, "test/mock")[0])
        self.assertFalse(integration.backend_auth_status(backend)[0])
        self.assertFalse(integration.backend_auth_status(backend, "test/different")[0])
        accepted["observedGeneration"] = 1
        self.assertEqual(integration.backend_auth_status(backend, "test/mock"), (False, ""))
        accepted["observedGeneration"] = 2
        accepted["reason"] = "Accepted"
        self.assertTrue(integration.backend_auth_status(backend)[0])

    def test_missing_gateway_address_has_clear_error(self):
        for gateway in ({}, {"status": None}, {"status": {"addresses": []}},
                        {"status": {"addresses": [{}]}}, {"status": {"addresses": [{"value": ""}]}}):
            with self.subTest(gateway=gateway), \
                    mock.patch.object(sys, "argv", ["runner", "--model", "model"]), \
                    mock.patch.object(integration.subprocess, "check_output", return_value=json.dumps(gateway).encode()) as request:
                with self.assertRaisesRegex(ValueError, "complete the ingress deployment"):
                    integration.main()
                self.assertEqual(request.call_count, 1)

    def test_skips_terminating_unready_and_addressless_proxy_pods(self):
        healthy = {"spec": {"serviceAccountName": "agentgateway-proxy"}, "status": {
            "phase": "Running", "podIP": "10.4.0.9", "conditions": [{"type": "Ready", "status": "True"}]}}
        terminating, addressless, unready = [copy.deepcopy(healthy) for _ in range(3)]
        terminating["metadata"] = {"deletionTimestamp": "2026-10-28T00:00:00Z"}
        addressless["status"].pop("podIP")
        unready["status"]["conditions"] = []
        for usable in (True, False):
            responses = [
                {"status": {"addresses": [{"value": "10.0.0.1"}]}}, {"items": []}, {"spec": {"type": "ClusterIP"}},
                {"items": [terminating, addressless, unready] + ([healthy] if usable else [])},
                {"metadata": {"annotations": {"iam.gke.io/gcp-service-account": "proxy@example.com"}}},
            ]
            jobs = []
            def kubectl(command, input=None, **kwargs):
                if command[3] == "get":
                    return json.dumps(responses.pop(0) if responses else {"status": {"succeeded": 1}}).encode()
                if command[3] == "create":
                    doc = json.loads(input)
                    if doc["kind"] == "Job":
                        jobs.append(doc)
                return b""
            with self.subTest(usable=usable), mock.patch.object(sys, "argv", ["runner", "--model", "model"]), \
                    mock.patch.object(integration.subprocess, "check_output", side_effect=kubectl):
                if usable:
                    integration.main()
                    self.assertIn("http://10.4.0.9:15020/metrics", jobs[0]["spec"]["template"]["spec"]["containers"][0]["args"])
                else:
                    with self.assertRaisesRegex(ValueError, "No ready, non-terminating"):
                        integration.main()
                    self.assertEqual(jobs, [])

    def test_no_running_proxy_has_clear_error(self):
        responses = [
            {"status": {"addresses": [{"value": "10.0.0.1"}]}},
            {"items": []}, {"spec": {"type": "ClusterIP"}},
            {"items": [{"status": None}]},
        ]
        with mock.patch.object(sys, "argv", ["runner", "--model", "model"]), \
                mock.patch.object(integration.subprocess, "check_output",
                                  side_effect=[json.dumps(item).encode() for item in responses]):
            with self.assertRaisesRegex(ValueError, "No ready, non-terminating agentgateway-proxy"):
                integration.main()

    def test_synthetic_success_and_failed_job_cleanup(self):
        for failed, cleanup_failure in ((False, 0), (True, 0), (False, 1), (True, 1), (False, 3), (True, 3)):
            documents, deleted, backend_checks = [], [], []

            def kubectl(command, input=None, **kwargs):
                args = command[3:]
                if args[0] == "create":
                    document = json.loads(input)
                    if document["kind"] == "Secret":
                        self.assertEqual(len(backend_checks), 1)
                    documents.append(document)
                    return b"created"
                if args[0] == "delete":
                    deleted.append((args, json.loads(input) if input else None))
                    if args[1] == "-f":
                        self.assertIn("--wait=true", args)
                        self.assertIn("--cascade=foreground", args)
                        self.assertIn("--timeout=120s", args)
                        self.assertEqual(kwargs["timeout"], 150)
                    else:
                        self.assertIn("--wait=false", args)
                    if cleanup_failure and len(deleted) == cleanup_failure:
                        raise integration.subprocess.TimeoutExpired(command, 60)
                    return b"deleted"
                if args[0] == "logs":
                    if "--all-containers" in args:
                        raise integration.subprocess.CalledProcessError(1, command, output=b"logs unavailable")
                    return b"test output"
                if args[0] != "get":
                    return b""
                kind = args[1]
                if kind == "agentgatewaybackend":
                    self.assertEqual(any(d["kind"] == "Secret" for d in documents), bool(backend_checks))
                    backend_checks.append(True)
                    return json.dumps({"status": {"conditions": [{"type": "Accepted", "status":
                        "False" if len(backend_checks) == 1 else "True"}]}}).encode()
                data = {
                    "gateway": {"status": {"addresses": [{"value": "10.1.0.5"}]}},
                    "services": {"items": []}, "service": {"spec": {"type": "ClusterIP"}},
                    "pods": {"items": [{"spec": {"serviceAccountName": "agentgateway-proxy"},
                        "status": {"phase": "Running", "podIP": "10.4.0.5",
                        "conditions": [{"type": "Ready", "status": "True"}]}}]},
                    "serviceaccount": {"metadata": {"annotations": {"iam.gke.io/gcp-service-account": "proxy@example.com"}}},
                    "httproute": {"metadata": {"generation": 1}, "status": {"parents": [{
                        "parentRef": {"name": "agentgateway-proxy"},
                        "conditions": GatewayReadinessTests.conditions("Accepted", "ResolvedRefs")}]}},
                    "job": {"status": {"failed" if failed else "succeeded": 1}},
                    "endpointslices": {"items": []},
                }
                return json.dumps(data[kind]).encode()

            with self.subTest(failed=failed, cleanup_failure=cleanup_failure), \
                    mock.patch.object(sys, "argv", ["run_in_cluster.py", "--mock-provider"]), \
                    mock.patch.object(integration.subprocess, "check_output", side_effect=kubectl), \
                    mock.patch.object(integration.time, "sleep"), \
                    mock.patch.object(sys, "stderr", io.StringIO()) as diagnostics:
                if failed:
                    with self.assertRaisesRegex(RuntimeError, "Smoke client failed"):
                        integration.main()
                    self.assertIn("logs unavailable", diagnostics.getvalue())
                elif cleanup_failure:
                    with self.assertRaisesRegex(RuntimeError, "cleanup failed"):
                        integration.main()
                else:
                    integration.main()
            self.assertEqual(len(backend_checks), 2)
            self.assertEqual(len(deleted), 3)
            self.assertEqual(deleted[-1][1]["items"][0]["kind"], "Secret")
            job = next(item for item in documents if item["kind"] == "Job")
            self.assertEqual(job["spec"]["template"]["spec"]["initContainers"][0]["name"], "wait-for-provider")


class ExposureTests(unittest.TestCase):
    def test_unsafe_configuration_fails_before_job_creation(self):
        safe = {
            "gateway": {"status": {"addresses": [{"value": "10.1.0.5"}]}},
            "services": {"items": []}, "service": {"spec": {"type": "ClusterIP"}},
            "pods": {"items": [{"spec": {"serviceAccountName": "agentgateway-proxy"},
                "status": {"phase": "Running", "podIP": "10.4.0.5",
                        "conditions": [{"type": "Ready", "status": "True"}]}}]},
            "serviceaccount": {"metadata": {"annotations": {"iam.gke.io/gcp-service-account": "proxy@example.com"}}},
            "job": {"status": {"succeeded": 1}},
        }
        cases = [
            ("gateway", {"status": {"addresses": [{"value": "8.8.8.8"}]}}, "internal load-balancer"),
            ("services", {"items": [{"spec": {"type": "LoadBalancer"}}]}, "externally exposed"),
            ("services", {"items": [{"spec": {"type": "ClusterIP", "externalIPs": ["8.8.8.8"]}}]}, "externally exposed"),
            ("service", {"spec": {"type": "NodePort"}}, "ClusterIP"),
            ("pods", {"items": [{"spec": {"serviceAccountName": "default"}, "status": {"phase": "Running", "podIP": "10.4.0.5",
                "conditions": [{"type": "Ready", "status": "True"}]}}]}, "service account"),
            ("serviceaccount", {"metadata": {}}, "Workload Identity"),
        ]
        for kind, invalid, message in cases:
            objects = dict(safe, **{kind: invalid})
            def kubectl(command, **kwargs):
                args = command[3:]
                if args[0] == "create":
                    self.fail("Unsafe configuration created a client Job")
                return json.dumps(objects[args[1]]).encode() if args[0] == "get" else b""
            with self.subTest(kind=kind, message=message), \
                    mock.patch.object(sys, "argv", ["runner", "--model", "model"]), \
                    mock.patch.object(integration.subprocess, "check_output", side_effect=kubectl):
                with self.assertRaisesRegex(ValueError, message):
                    integration.main()


class StaticValidationTests(unittest.TestCase):
    def test_missing_chart_schemas_fail(self):
        schemas = {("agentgateway.dev/v1alpha1", kind): {}
                   for kind in ("AgentgatewayBackend", "AgentgatewayParameters")}
        static.require_schemas(schemas)
        for missing in schemas:
            with self.subTest(missing=missing), self.assertRaisesRegex(ValueError, "missing required CRD"):
                static.require_schemas({k: v for k, v in schemas.items() if k != missing})
        with self.assertRaisesRegex(ValueError, "missing required CRD"):
            static.require_schemas({})

    def test_empty_chart_render_fails_before_template_rendering(self):
        with mock.patch.object(static.subprocess, "check_output", return_value=b""), \
                mock.patch.object(static, "render") as render:
            with self.assertRaisesRegex(ValueError, "missing required CRD"):
                static.main()
            render.assert_not_called()

    def test_gateway_checks_have_specific_diagnostics(self):
        for optional in (False, True):
            paths = {"vertex-ai": "/v1/chat/completions"}
            if optional:
                paths.update(openai="/openai/v1/chat/completions", anthropic="/v1/messages")
            resources = [{"kind": "AgentgatewayBackend", "metadata": {"name": name}} for name in paths]
            for name, path in paths.items():
                rule = {"matches": [{"path": {"type": "Exact", "value": path}}]}
                if name == "openai":
                    rule["filters"] = [{"type": "URLRewrite", "urlRewrite": {"path": {
                        "type": "ReplaceFullPath", "replaceFullPath": "/v1/chat/completions"}}}]
                resources.append({"kind": "HTTPRoute", "metadata": {"name": name}, "spec": {"rules": [rule]}})
            static.validate_gateway(list(reversed(resources)), optional)
            def route(items, name):
                return next(r for r in items if r["kind"] == "HTTPRoute" and r["metadata"]["name"] == name)["spec"]["rules"][0]
            cases = [
                ("number of agentgateway backends", lambda items: items.append({"kind": "AgentgatewayBackend"})),
                ("must not create Secrets", lambda items: items.append({"kind": "Secret"})),
                ("route names", lambda items: items.append({"kind": "HTTPRoute", "metadata": {"name": "extra"}, "spec": {"rules": [{}]}})),
                ("path match for route vertex-ai", lambda items: route(items, "vertex-ai").update(matches=[])),
                ("rewrite filter on route vertex-ai", lambda items: route(items, "vertex-ai").update(filters=[])),
            ]
            if optional:
                cases.append(("OpenAI path rewrite", lambda items: route(items, "openai").update(filters=[])))
            for message, mutate in cases:
                broken = copy.deepcopy(resources)
                mutate(broken)
                with self.subTest(optional=optional, failure=message), self.assertRaisesRegex(ValueError, message):
                    static.validate_gateway(broken, optional)

    def test_ingress_requires_resources_and_correct_wiring(self):
        # Only scalar namespace interpolation is needed to inspect the ingress resources;
        # the readiness manifest is validated separately by the chart check.
        text = (ROOT / "files/ingress.yaml.tftpl").read_text().split("${readiness_manifest}")[0]
        import yaml
        resources = [r for r in yaml.safe_load_all(text.replace('${jsonencode(namespace)}', 'test')) if r]
        static.validate_ingress(resources)
        for i in range(len(resources)):
            with self.subTest(missing=resources[i]["kind"]), self.assertRaisesRegex(ValueError, "missing expected"):
                static.validate_ingress(resources[:i] + resources[i+1:])
        broken = copy.deepcopy(resources)
        next(r for r in broken if r["kind"] == "Gateway")["spec"]["gatewayClassName"] = "gke-l7-global-external-managed"
        with self.assertRaisesRegex(ValueError, "internal GatewayClass"):
            static.validate_ingress(broken)
        broken = copy.deepcopy(resources)
        next(r for r in broken if r["kind"] == "HTTPRoute")["spec"]["rules"][0]["backendRefs"][0]["port"] = 80
        with self.assertRaisesRegex(ValueError, "proxy backend"):
            static.validate_ingress(broken)



class AnthropicMessagesTests(unittest.TestCase):
    TEXT = {"type": "content_block_delta", "index": 0,
            "delta": {"type": "text_delta", "text": "one"}}
    FINISH = {"type": "message_delta", "delta": {"stop_reason": "end_turn"}}
    STOP = {"type": "message_stop"}
    MESSAGE = {"type": "message", "role": "assistant", "stop_reason": "end_turn",
               "content": [{"type": "text", "text": "one"}]}

    def test_native_requests_and_responses(self):
        sync = io.BytesIO(json.dumps(self.MESSAGE).encode())
        stream = response([{"type": "message_start", "message": {"content": []}},
                           {"type": "ping"}, self.TEXT, self.FINISH, self.STOP])
        with mock.patch.object(smoke, "open_response", side_effect=[sync, stream]) as send:
            smoke.smoke("http://gateway/", "claude-model", max_output_tokens=256, api_format="anthropic")
        for call, streaming in zip(send.call_args_list, (False, True)):
            request = call.args[0]
            self.assertEqual(request.full_url, "http://gateway/v1/messages")
            self.assertEqual(request.get_header("Anthropic-version"), "2023-06-01")
            self.assertEqual(json.loads(request.data), {
                "model": "claude-model", "messages": [{"role": "user", "content": "Count from one to ten."}],
                "max_tokens": 256, "stream": streaming})

    def test_bad_synchronous_responses(self):
        for result in ({"choices": [{"message": {"content": "wrong format"}}]},
                       dict(self.MESSAGE, content=[]), dict(self.MESSAGE, stop_reason=None),
                       dict(self.MESSAGE, error={"message": "private provider details"}),
                       dict(self.MESSAGE, content=[{"type": "thinking", "thinking": "no visible text"}])):
            with self.subTest(result=result), mock.patch.object(smoke, "open_response",
                    return_value=io.BytesIO(json.dumps(result).encode())):
                with self.assertRaisesRegex(ValueError, "Missing synchronous completion"):
                    smoke.smoke("http://gateway", "claude-model", api_format="anthropic")

    def test_bad_streams(self):
        cases = [([self.TEXT, self.FINISH], "without message_stop"),
                 ([self.FINISH, self.STOP], "without content"),
                 ([self.TEXT, self.STOP], "stop reason"),
                 ([self.TEXT, {"type": "error", "error": {"message": "secret"}}], "SSE error"),
                 ([CONTENT, FINISH], "without message_stop")]
        for events, error in cases:
            with self.subTest(events=events), self.assertRaisesRegex(ValueError, error):
                smoke.check_stream(response(events), "anthropic")

    def test_stream_ignores_nontext_and_unknown_events(self):
        events = [{"type": "ping"}, {"type": "future_event"},
                  {"type": "content_block_delta", "delta": {"type": "thinking_delta", "thinking": "hidden"}},
                  self.TEXT, self.FINISH, self.STOP]
        self.assertEqual(smoke.check_stream(response(events), "anthropic"), 1)

    def test_rejects_wrong_content_type(self):
        with self.assertRaisesRegex(ValueError, "text/event-stream"):
            smoke.check_stream(response([], "application/json"), "anthropic")

    def test_token_parameter_rejected_before_requests(self):
        for module, args in ((smoke, ["--endpoint", "http://gateway", "--api-format", "anthropic"]),
                             (integration, ["--provider", "anthropic"])):
            with self.subTest(module=module.__name__), mock.patch("sys.stderr", io.StringIO()):
                with self.assertRaises(SystemExit):
                    module.parse_args(args + ["--model", "claude-model", "--token-parameter", "max_completion_tokens"])

    def test_runner_rejects_api_override(self):
        with mock.patch("sys.stderr", io.StringIO()), self.assertRaises(SystemExit):
            integration.parse_args(["--mock-provider", "--api-format", "anthropic"])


if __name__ == "__main__":
    unittest.main()
