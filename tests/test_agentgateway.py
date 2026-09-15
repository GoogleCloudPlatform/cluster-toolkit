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

        def add_option(parser):
            original(parser)
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
        for provider, suffix in ((None, ""), ("openai", "/openai"), ("anthropic", "/anthropic")):
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

                argv = ["run-in-cluster.py", "--model", "test-model", "--token-parameter",
                        "max_completion_tokens", "--max-output-tokens", "2048"]
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
                self.assertEqual(client.token_parameter, "max_completion_tokens")
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
                 ("/anthropic", "test-anthropic", "max_tokens", 256),
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


if __name__ == "__main__":
    unittest.main()
