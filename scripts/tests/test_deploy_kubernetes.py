import argparse
import contextlib
import importlib.util
import io
import json
import pathlib
import subprocess
import unittest
from unittest.mock import patch

path = pathlib.Path(__file__).resolve().parents[1] / "deploy-kubernetes.py"
spec = importlib.util.spec_from_file_location("release", path)
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class MigrationGateTests(unittest.TestCase):
    def setUp(self):
        self.args = argparse.Namespace(context="test-only", namespace="test", deployment="urlo", container="backend",
            image="ghcr.io/kongken/urlo@sha256:" + "a" * 64, migration_secret="migration-role",
            secret_key="dsn", pg_ca_secret="postgres-ca", migration_timeout=300, rollout_timeout=300)
        self.deployment = {"metadata": {"uid": "original-deployment"}, "spec": {"template": {
            "metadata": {"labels": {"app": "urlo"}}, "spec": {"imagePullSecrets": [{"name": "registry"}],
                "containers": [{"name": "backend", "image": "old"}, {"name": "sidecar", "image": "unchanged"}]}}}}
        self.calls = []

    def run_release(self, condition="Complete", fail=None):
        def cli(args, *command, payload=None):
            self.calls.append((command, payload))
            if fail == command[0]:
                raise RuntimeError("simulated API failure")
            if command[:2] == ("get", "deployment"):
                return json.dumps(self.deployment)
            if command[:2] == ("get", "job"):
                return json.dumps({"status": {"conditions": [{"type": condition, "status": "True"}]}})
            return ""
        with patch.object(release, "kubectl", side_effect=cli), contextlib.redirect_stdout(io.StringIO()):
            release.deploy(self.args)

    def test_complete_job_gates_exact_image_and_preserves_other_containers(self):
        self.run_release()
        self.assertEqual([c[0][0] for c in self.calls], ["get", "create", "get", "patch", "rollout"])
        job = self.calls[1][1]
        pod = job["spec"]["template"]["spec"]
        self.assertFalse(pod["automountServiceAccountToken"])
        self.assertEqual(pod["imagePullSecrets"], [{"name": "registry"}])
        migration = pod["containers"][0]
        self.assertEqual(migration["image"], self.args.image)
        self.assertEqual(migration["command"], ["/app/urlo-migrate"])
        self.assertEqual(migration["env"][0]["valueFrom"]["secretKeyRef"], {"name": "migration-role", "key": "dsn"})
        self.assertNotIn("value", migration["env"][0])
        self.assertEqual(job["spec"]["backoffLimit"], 0)
        operations = self.calls[3][1]
        self.assertEqual(operations[0]["path"], "/metadata/uid")
        self.assertEqual(operations[1]["path"], "/spec/template")
        self.assertEqual(operations[2]["value"], self.args.image)
        self.assertEqual(operations[2]["path"], "/spec/template/spec/containers/0/image")
        self.assertEqual(self.deployment["spec"]["template"]["spec"]["containers"][1]["image"], "unchanged")
        self.assertIn("--patch-file=/dev/stdin", self.calls[3][0])

    def test_failed_job_leaves_deployment_unchanged(self):
        with self.assertRaisesRegex(RuntimeError, "failed; Deployment left unchanged"):
            self.run_release(condition="Failed")
        self.assertFalse(any(c[0][0] == "patch" for c in self.calls))

    def test_pending_job_timeout_blocks_rollout(self):
        with patch.object(release.time, "monotonic", side_effect=[0, 1000]):
            with self.assertRaisesRegex(RuntimeError, "timed out"):
                self.run_release(condition="Pending")
        self.assertFalse(any(c[0][0] == "patch" for c in self.calls))

    def test_job_creation_failure_blocks_rollout(self):
        with self.assertRaises(RuntimeError):
            self.run_release(fail="create")
        self.assertFalse(any(c[0][0] == "patch" for c in self.calls))

    def test_concurrent_template_change_patch_failure_stops_release(self):
        with self.assertRaises(RuntimeError):
            self.run_release(fail="patch")
        self.assertFalse(any(c[0][0] == "rollout" for c in self.calls))

    def test_unknown_container_does_not_start_job(self):
        self.args.container = "missing"
        with self.assertRaisesRegex(RuntimeError, "container"):
            self.run_release()
        self.assertEqual(len(self.calls), 1)

    def test_immutable_digest_and_explicit_target_required(self):
        args = ["deploy", "--context", "test", "--namespace", "test", "--deployment", "urlo",
                "--container", "backend", "--migration-secret", "migration-role", "--image", "repo:latest"]
        with patch.object(release.sys, "argv", args), contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit):
                release.arguments()

    def test_cluster_error_output_not_echoed(self):
        response = subprocess.CompletedProcess([], 1, stdout="secret-output", stderr="password-secret")
        with patch.object(release.subprocess, "run", return_value=response):
            with self.assertRaises(RuntimeError) as caught:
                release.kubectl(self.args, "get", "deployment", "urlo")
        self.assertNotIn("secret", str(caught.exception))


if __name__ == "__main__":
    unittest.main()
