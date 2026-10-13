#!/usr/bin/env python3
"""Explicit release gate: migrate the exact image before patching a Deployment."""
import argparse
import json
import re
import subprocess
import sys
import time
import uuid


def arguments():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("context", "namespace", "deployment", "container", "image", "migration-secret"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--secret-key", default="dsn")
    parser.add_argument("--pg-ca-secret", help="optional Secret containing ca.crt for PGSSLROOTCERT")
    parser.add_argument("--migration-timeout", type=int, default=300, help="seconds, including lock wait (1..3600)")
    parser.add_argument("--rollout-timeout", type=int, default=300)
    args = parser.parse_args()
    if not args.context.strip() or args.context != args.context.strip():
        parser.error("--context must explicitly name a nonempty kubeconfig context")
    if not re.fullmatch(r"[^\s@]+@sha256:[0-9a-f]{64}", args.image):
        parser.error("--image must be an immutable image@sha256:<64 lowercase hex> digest")
    for name in ("namespace", "deployment", "container", "migration_secret", "pg_ca_secret"):
        value = getattr(args, name)
        if value is not None and (len(value) > 63 or not re.fullmatch(r"[a-z0-9](?:[-a-z0-9]*[a-z0-9])?", value)):
            parser.error(f"invalid Kubernetes name: {name}")
    if not re.fullmatch(r"[A-Za-z0-9._-]+", args.secret_key):
        parser.error("invalid Secret key")
    if not 1 <= args.migration_timeout <= 3600 or not 1 <= args.rollout_timeout <= 3600:
        parser.error("timeouts must be between 1 and 3600 seconds")
    return args


def kubectl(args, *command, payload=None):
    result = subprocess.run(
        ["kubectl", "--context", args.context, "--namespace", args.namespace,
         f"--request-timeout={args.rollout_timeout + 5 if command[0] == 'rollout' else 10}s", *command],
        input=json.dumps(payload) if payload is not None else None,
        text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        timeout=args.rollout_timeout + 15 if command[0] == "rollout" else 15,
    )
    if result.returncode:
        # Do not dump arbitrary cluster output/Pod logs which may contain secrets.
        raise RuntimeError(f"kubectl {command[0]} failed (exit {result.returncode}); inspect cluster diagnostics")
    return result.stdout


def migration_job(args, deployment, name):
    source_spec = deployment["spec"]["template"]["spec"]
    container = {
        "name": "migrate", "image": args.image, "imagePullPolicy": "IfNotPresent",
        "command": ["/app/urlo-migrate"], "args": [f"--timeout={args.migration_timeout}s"],
        "env": [{"name": "URLO_POSTGRES_DSN", "valueFrom": {"secretKeyRef": {
            "name": args.migration_secret, "key": args.secret_key}}}],
        "resources": {"requests": {"cpu": "100m", "memory": "64Mi"},
                      "limits": {"cpu": "1", "memory": "256Mi"}},
        "securityContext": {"runAsNonRoot": True, "runAsUser": 10001, "runAsGroup": 10001,
                            "readOnlyRootFilesystem": True, "allowPrivilegeEscalation": False,
                            "capabilities": {"drop": ["ALL"]}, "seccompProfile": {"type": "RuntimeDefault"}},
    }
    pod = {"restartPolicy": "Never", "automountServiceAccountToken": False, "containers": [container]}
    for key in ("imagePullSecrets", "nodeSelector", "tolerations"):
        if key in source_spec:
            pod[key] = source_spec[key]
    if args.pg_ca_secret:
        pod["volumes"] = [{"name": "postgres-ca", "secret": {"secretName": args.pg_ca_secret}}]
        container["volumeMounts"] = [{"name": "postgres-ca", "mountPath": "/etc/postgresql", "readOnly": True}]
        container["env"].append({"name": "PGSSLROOTCERT", "value": "/etc/postgresql/ca.crt"})
    return {"apiVersion": "batch/v1", "kind": "Job",
            "metadata": {"name": name, "namespace": args.namespace, "labels": {"app.kubernetes.io/name": "urlo-migrate"}},
            "spec": {"backoffLimit": 0, "activeDeadlineSeconds": args.migration_timeout + 30,
                     "ttlSecondsAfterFinished": 3600, "template": {"spec": pod}}}


def deploy(args):
    deployment = json.loads(kubectl(args, "get", "deployment", args.deployment, "-o", "json"))
    template = deployment["spec"]["template"]
    positions = [i for i, c in enumerate(template["spec"]["containers"]) if c["name"] == args.container]
    if len(positions) != 1:
        raise RuntimeError("target container does not exist exactly once; no Job or rollout performed")
    job_name = f"urlo-migrate-{uuid.uuid4().hex[:16]}"
    kubectl(args, "create", "-f", "-", payload=migration_job(args, deployment, job_name))
    print(f"Waiting for migration Job {job_name}", flush=True)
    deadline = time.monotonic() + args.migration_timeout + 60
    while True:
        job = json.loads(kubectl(args, "get", "job", job_name, "-o", "json"))
        conditions = {c["type"]: c["status"] for c in job.get("status", {}).get("conditions", [])}
        if conditions.get("Failed") == "True":
            raise RuntimeError(f"migration Job {job_name} failed; Deployment left unchanged")
        if conditions.get("Complete") == "True":
            break
        if time.monotonic() >= deadline:
            raise RuntimeError(f"migration Job {job_name} timed out; Deployment left unchanged")
        time.sleep(2)
    # Optimistic guards prevent concurrent releases/config edits from being
    # silently overwritten. Status-only changes do not invalidate the template.
    patch = [
        {"op": "test", "path": "/metadata/uid", "value": deployment["metadata"]["uid"]},
        {"op": "test", "path": "/spec/template", "value": template},
        {"op": "replace", "path": f"/spec/template/spec/containers/{positions[0]}/image", "value": args.image},
    ]
    kubectl(args, "patch", "deployment", args.deployment, "--type=json", "--patch-file=/dev/stdin", payload=patch)
    kubectl(args, "rollout", "status", "deployment/" + args.deployment, f"--timeout={args.rollout_timeout}s")
    print(f"Release complete: {args.deployment}/{args.container} -> {args.image}")


if __name__ == "__main__":
    try:
        deploy(arguments())
    except (RuntimeError, OSError, subprocess.TimeoutExpired, ValueError, KeyError) as error:
        print(f"Release stopped: {error}", file=sys.stderr)
        sys.exit(1)
