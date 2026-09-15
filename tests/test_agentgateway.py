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
import importlib.util
import io
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


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


smoke = load("smoke", ROOT / "smoke-test.py")
readiness = load("readiness", ROOT / "files/readiness.py")
http_readiness = load("http_readiness", Path(__file__).resolve().parents[1] /
                      "tools/cloud-build/daily-tests/ansible_playbooks/test-validation/agentgateway/wait-for-http.py")

integration = load("integration", Path(__file__).resolve().parents[1] /
                   "tools/cloud-build/daily-tests/ansible_playbooks/test-validation/agentgateway/run-in-cluster.py")


class ProviderSelectionTests(unittest.TestCase):
    def test_forwarded_defaults_match_direct_client(self):
        args = integration.parse_args(["--model", "test-model"])
        direct = smoke.parse_args(["--endpoint", "http://gateway"] + args.client_arguments)
        self.assertEqual(args.model, direct.model)
        self.assertEqual(args.token_parameter, direct.token_parameter)
        self.assertEqual(args.max_output_tokens, direct.max_output_tokens)

    def test_new_client_option_is_forwarded_without_runner_changes(self):
        original = integration.smoke_client.add_client_arguments

        def add_option(parser, **kwargs):
            original(parser, **kwargs)
            parser.add_argument("--test-client-option")

        with mock.patch.object(integration.smoke_client, "add_client_arguments", side_effect=add_option):
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
                                              "status": {"phase": "Running", "podIP": "10.4.0.5"}}]},
                        "serviceaccount": {"metadata": {"annotations": {"iam.gke.io/gcp-service-account": "proxy@example.com"}}},
                        "job": {"status": {"succeeded": 1}},
                    }
                    return json.dumps(objects[kind]).encode()

                parameter = "max_tokens" if provider == "anthropic" else "max_completion_tokens"
                argv = ["run-in-cluster.py", "--model", "test-model", "--token-parameter",
                        parameter, "--max-output-tokens", "2048"]
                if provider:
                    argv += ["--provider", provider]
                with mock.patch.object(integration.sys, "argv", argv), \
                        mock.patch.object(integration.subprocess, "check_output", side_effect=kubectl):
                    integration.main()
                self.assertEqual(len(jobs), 1)
                self.assertEqual(configs[0]["data"]["smoke-test.py"], (ROOT / "smoke-test.py").read_text())
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
                    mock.patch.object(integration.sys, "argv", ["run-in-cluster.py", "--model", "test",
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
        provider = load("mock_provider", Path(__file__).resolve().parents[1] / "tools/cloud-build/daily-tests/ansible_playbooks/test-validation/agentgateway/mock-provider.py")
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
        errors = [urllib.error.HTTPError("http://gateway", 503, "unavailable", {}, None),
                  urllib.error.URLError("connection refused"), TimeoutError("socket timeout"), result]
        with mock.patch.object(smoke.urllib.request, "urlopen", side_effect=errors) as request, \
                mock.patch.object(smoke.time, "sleep"):
            self.assertIs(smoke.open_response("http://gateway"), result)
        self.assertEqual(request.call_count, 4)

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
                    mock.patch.object(sys, "argv", ["smoke-test.py", "--endpoint", "http://gateway",
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

    def test_ready_gateway_and_ingress(self):
        for stage in ("gateway", "ingress"):
            objects = self.objects(stage)
            with self.subTest(stage=stage):
                self.assertTrue(readiness.ready(lambda path: next(value for key, value in objects.items()
                                                                 if path.endswith('/' + key)), "test", stage))

    def test_incomplete_route_backend_and_deployment_are_not_ready(self):
        for missing in ("parents", "conditions", "backend", "deployment"):
            objects = self.objects("gateway")
            route = objects["httproutes"]["items"][0]
            if missing == "parents":
                route["status"]["parents"] = []
            elif missing == "conditions":
                route["status"]["parents"][0]["conditions"] = []
            elif missing == "backend":
                objects["agentgatewaybackends"]["items"] = []
            else:
                objects["deployments/agentgateway-proxy"]["status"]["observedGeneration"] = 0
            with self.subTest(missing=missing):
                self.assertFalse(readiness.ready(lambda path: next(value for key, value in objects.items()
                                                                  if path.endswith('/' + key)), "test", "gateway"))

    def test_gateway_api_requires_established_crds(self):
        crd = {"metadata": {"annotations": {"gateway.networking.k8s.io/bundle-version": "v1.5.0"}},
               "status": {"conditions": self.conditions("Established", "Accepted")}}
        self.assertTrue(readiness.ready(lambda _: crd, "test", "gateway-api"))
        crd["status"]["conditions"] = []
        self.assertFalse(readiness.ready(lambda _: crd, "test", "gateway-api"))

    def test_readiness_job_retries_transient_api_failure(self):
        result = io.BytesIO(b'{"status":{"conditions":[]}}')
        def ready(get, namespace, stage):
            get('/test')
            return True
        with mock.patch.object(readiness.ssl, "create_default_context"), \
                mock.patch.object(readiness.Path, "read_text", return_value="test-token"), \
                mock.patch.dict(readiness.os.environ, {"STAGE": "gateway", "NAMESPACE": "test"}), \
                mock.patch.object(readiness, "ready", side_effect=ready), \
                mock.patch.object(readiness.urllib.request, "urlopen", side_effect=[
                    urllib.error.HTTPError("http://api", 503, "unavailable", {}, None), result]), \
                mock.patch.object(readiness.time, "sleep"):
            readiness.main()


class RunnerLifecycleTests(unittest.TestCase):
    def test_no_running_proxy_has_clear_error(self):
        responses = [
            {"status": {"addresses": [{"value": "10.0.0.1"}]}},
            {"items": []}, {"spec": {"type": "ClusterIP"}},
            {"items": [{"status": None}]},
        ]
        with mock.patch.object(sys, "argv", ["runner", "--model", "model"]), \
                mock.patch.object(integration.subprocess, "check_output",
                                  side_effect=[json.dumps(item).encode() for item in responses]):
            with self.assertRaisesRegex(ValueError, "No running agentgateway-proxy"):
                integration.main()

    def test_synthetic_success_and_failed_job_cleanup(self):
        for failed in (False, True):
            documents, deleted, backend_checks = [], [], []

            def kubectl(command, input=None, **kwargs):
                args = command[3:]
                if args[0] == "create":
                    documents.append(json.loads(input))
                    return b"created"
                if args[0] == "delete":
                    deleted.append((args, json.loads(input) if input else None))
                    return b"deleted"
                if args[0] == "logs":
                    if "--all-containers" in args:
                        raise integration.subprocess.CalledProcessError(1, command, output=b"logs unavailable")
                    return b"test output"
                if args[0] != "get":
                    return b""
                kind = args[1]
                if kind == "agentgatewaybackend":
                    backend_checks.append(True)
                    return json.dumps({"status": {"conditions": [{"type": "Accepted", "status":
                        "False" if len(backend_checks) == 1 else "True"}]}}).encode()
                data = {
                    "gateway": {"status": {"addresses": [{"value": "10.1.0.5"}]}},
                    "services": {"items": []}, "service": {"spec": {"type": "ClusterIP"}},
                    "pods": {"items": [{"spec": {"serviceAccountName": "agentgateway-proxy"},
                        "status": {"phase": "Running", "podIP": "10.4.0.5"}}]},
                    "serviceaccount": {"metadata": {"annotations": {"iam.gke.io/gcp-service-account": "proxy@example.com"}}},
                    "httproute": {"metadata": {"generation": 1}, "status": {"parents": [{
                        "conditions": GatewayReadinessTests.conditions("Accepted", "ResolvedRefs")}]}},
                    "job": {"status": {"failed" if failed else "succeeded": 1}},
                    "endpointslices": {"items": []},
                }
                return json.dumps(data[kind]).encode()

            with self.subTest(failed=failed), \
                    mock.patch.object(sys, "argv", ["run-in-cluster.py", "--mock-provider"]), \
                    mock.patch.object(integration.subprocess, "check_output", side_effect=kubectl), \
                    mock.patch.object(sys, "stderr", io.StringIO()) as diagnostics:
                if failed:
                    with self.assertRaisesRegex(RuntimeError, "Smoke client failed"):
                        integration.main()
                    self.assertIn("logs unavailable", diagnostics.getvalue())
                else:
                    integration.main()
            self.assertEqual(len(backend_checks), 2)
            self.assertEqual(len(deleted), 3)
            self.assertEqual(deleted[-1][1]["items"][0]["kind"], "Secret")
            job = next(item for item in documents if item["kind"] == "Job")
            self.assertEqual(job["spec"]["template"]["spec"]["initContainers"][0]["name"], "wait-for-provider")


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
        with mock.patch.object(smoke, "open_response") as send:
            with self.assertRaisesRegex(ValueError, "max_tokens"):
                smoke.smoke("http://gateway", "claude-model", "max_completion_tokens", api_format="anthropic")
            send.assert_not_called()

    def test_runner_rejects_api_override(self):
        with mock.patch("sys.stderr", io.StringIO()), self.assertRaises(SystemExit):
            integration.parse_args(["--mock-provider", "--api-format", "anthropic"])


if __name__ == "__main__":
    unittest.main()
