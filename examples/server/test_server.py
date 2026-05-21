#!/usr/bin/env python3
import json
import os
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.abspath(__file__))
ARD = os.environ.get("ARD", "ard")
BIN = os.path.join(ROOT, "server")


def free_port():
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()
    return port


def request(method, url, body=None):
    data = None
    headers = {}
    if body is not None:
        data = body.encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=2) as resp:
            return resp.status, resp.read().decode("utf-8").rstrip("\n")
    except urllib.error.HTTPError as err:
        return err.code, err.read().decode("utf-8").rstrip("\n")


def wait_for_server(base_url, proc, timeout=30.0):
    deadline = time.time() + timeout
    last_error = None
    while time.time() < deadline:
        if proc.poll() is not None:
            stdout, stderr = proc.communicate()
            raise AssertionError(
                f"server exited before becoming ready with status {proc.returncode}\n"
                f"stdout:\n{stdout}\nstderr:\n{stderr}"
            )
        try:
            request("GET", base_url + "/")
            return
        except Exception as err:
            last_error = err
            time.sleep(0.1)
    raise AssertionError(f"server at {base_url} did not become ready; last error: {last_error}")


def assert_response(method, url, body, want_status, want_body):
    status, got_body = request(method, url, body)
    if status != want_status or got_body != want_body:
        raise AssertionError(
            f"{method} {url} = ({status}, {got_body!r}), want ({want_status}, {want_body!r})"
        )


def build():
    subprocess.run([ARD, "build", "--out", "server", "main.ard"], cwd=ROOT, check=True)


def main():
    build()
    port = free_port()
    env = os.environ.copy()
    env["PORT"] = str(port)
    proc = subprocess.Popen(
        [BIN],
        cwd=ROOT,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    try:
        base_url = f"http://127.0.0.1:{port}"
        wait_for_server(base_url, proc)
        assert_response("GET", base_url + "/", None, 200, "Hello, World!")
        assert_response("GET", base_url + "/me", None, 200, "this is /me")
        assert_response("GET", base_url + "/error", None, 400, "Bad request")
        assert_response(
            "POST",
            base_url + "/api/auth/sign-up",
            json.dumps({"email": "ard@example.com"}),
            201,
            "Created user with email ard@example.com",
        )
        assert_response("POST", base_url + "/api/auth/sign-up", "", 400, "Missing request body")
        assert_response(
            "POST",
            base_url + "/api/auth/sign-up",
            json.dumps({"name": "Ard"}),
            400,
            'Missing email: email: got Missing field "email", expected Field',
        )
        print("server smoke test passed")
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()


if __name__ == "__main__":
    try:
        main()
    except Exception as err:
        print(f"FAIL: {err}", file=sys.stderr)
        sys.exit(1)
