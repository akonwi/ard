#!/usr/bin/env python3
"""Manual LSP validation harness.

Drives the real ard LSP binary over stdio against examples/vaxis-demo,
printing per-request timings and a pass/fail summary. This is a manual
validation tool, not part of the automated test suite: it exercises the
real binary, the real wire protocol, and the real vaxis Go dependency
(network required for the first go/packages load).

Usage:
    cd compiler && go build -o /tmp/ard-lsp-test .
    python3 scripts/lsp-harness.py

Environment:
    ARD_LSP_BINARY  path to the built compiler binary (default /tmp/ard-lsp-test)
    ARD_LSP_DEMO    project root to open (default examples/vaxis-demo)

Track record: first runs caught two checker soundness bugs (untyped field
assignment, mut-parameter rejection), a server crash on malformed frames,
and a double-`mut` hover rendering bug that substring assertions in unit
tests had masked.
"""
import json
import os
import subprocess
import sys
import threading
import time
import queue

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
BINARY = os.environ.get("ARD_LSP_BINARY", "/tmp/ard-lsp-test")
DEMO = os.environ.get("ARD_LSP_DEMO", os.path.join(REPO, "examples", "vaxis-demo"))
MAIN = os.path.join(DEMO, "main.ard")

if not os.path.exists(BINARY):
    sys.exit(f"binary not found at {BINARY}; build with: cd compiler && go build -o {BINARY} .")
if not os.path.exists(MAIN):
    sys.exit(f"demo project not found at {DEMO}")

proc = subprocess.Popen([BINARY, "lsp"], stdin=subprocess.PIPE,
                        stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                        cwd=DEMO)

responses = queue.Queue()
notifications = queue.Queue()
stderr_lines = []

def read_stderr():
    for line in proc.stderr:
        stderr_lines.append(line.decode(errors="replace").rstrip())

def read_loop():
    buf = b""
    while True:
        # read headers
        header = b""
        while b"\r\n\r\n" not in header:
            ch = proc.stdout.read(1)
            if not ch:
                return
            header += ch
        length = 0
        for line in header.split(b"\r\n"):
            if line.lower().startswith(b"content-length:"):
                length = int(line.split(b":")[1].strip())
        body = b""
        while len(body) < length:
            chunk = proc.stdout.read(length - len(body))
            if not chunk:
                return
            body += chunk
        msg = json.loads(body)
        if "id" in msg and ("result" in msg or "error" in msg):
            responses.put(msg)
        else:
            notifications.put(msg)

threading.Thread(target=read_loop, daemon=True).start()
threading.Thread(target=read_stderr, daemon=True).start()

next_id = [0]

def send(method, params, notify=False):
    msg = {"jsonrpc": "2.0", "method": method, "params": params}
    if not notify:
        next_id[0] += 1
        msg["id"] = next_id[0]
    data = json.dumps(msg).encode()
    frame = b"Content-Length: %d\r\n\r\n%s" % (len(data), data)
    proc.stdin.write(frame)
    proc.stdin.flush()
    return msg.get("id")

def request(method, params, timeout=30):
    rid = send(method, params)
    start = time.monotonic()
    while True:
        try:
            msg = responses.get(timeout=timeout)
        except queue.Empty:
            print(f"  TIMEOUT ({timeout}s) waiting for {method}")
            return None, timeout * 1000
        if msg.get("id") == rid:
            elapsed = (time.monotonic() - start) * 1000
            return msg, elapsed
        # response for an older id; keep draining
    
def await_diagnostics(timeout=30, version=None):
    start = time.monotonic()
    while True:
        try:
            msg = notifications.get(timeout=timeout)
        except queue.Empty:
            return None, timeout * 1000
        if msg.get("method") == "textDocument/publishDiagnostics":
            params = msg["params"]
            if version is not None and params.get("version") != version:
                continue  # stale publish for an older document version
            return params, (time.monotonic() - start) * 1000

def drain_notifications():
    try:
        while True:
            notifications.get_nowait()
    except queue.Empty:
        pass

def doc_uri(path):
    return "file://" + path

results = []
def record(name, ok, ms, detail=""):
    status = "PASS" if ok else "FAIL"
    results.append((status, name, ms, detail))
    print(f"  [{status}] {name}: {ms:.0f}ms {detail}")

source = open(MAIN).read()

print("== initialize ==")
msg, ms = request("initialize", {
    "processId": os.getpid(),
    "rootUri": doc_uri(DEMO),
    "capabilities": {},
    "workspaceFolders": [{"uri": doc_uri(DEMO), "name": "vaxis-demo"}],
})
record("initialize", msg is not None and "result" in msg, ms)
send("initialized", {}, notify=True)

print("== didOpen (cold: real vaxis go/packages load) ==")
t0 = time.monotonic()
send("textDocument/didOpen", {"textDocument": {
    "uri": doc_uri(MAIN), "languageId": "ard", "version": 1, "text": source,
}}, notify=True)
diags, ms = await_diagnostics(timeout=120)
cold_ms = (time.monotonic() - t0) * 1000
n = len(diags["diagnostics"]) if diags else -1
record("cold diagnostics (clean file)", diags is not None and n == 0, cold_ms, f"{n} diagnostics")

def hover(line, char, label, expect_content=True, timeout=30):
    msg, ms = request("textDocument/hover", {
        "textDocument": {"uri": doc_uri(MAIN)},
        "position": {"line": line, "character": char},
    }, timeout=timeout)
    got = msg and msg.get("result")
    content = ""
    if got:
        content = got.get("contents", {}).get("value", "")[:60].replace("\n", " ")
    ok = (got is not None) == expect_content and (msg is not None and "error" not in msg)
    record(f"hover {label} ({line}:{char})", ok, ms, repr(content))

print("== hover (warm) ==")
hover(5, 8, "struct DemoState decl")        # struct DemoState {
hover(22, 7, "state var")                    # let state = ...
hover(23, 9, "state.ticks field")            # state.ticks
hover(33, 4, "fn set_page decl")             # fn set_page
hover(237, 3, "ui::Flex literal")            # ui::Flex{
hover(20, 12, "mut ffi::StateCtx param")     # c: mut ffi::StateCtx

print("== definition ==")
def definition(line, char, label, want_line=None):
    msg, ms = request("textDocument/definition", {
        "textDocument": {"uri": doc_uri(MAIN)},
        "position": {"line": line, "character": char},
    })
    locs = (msg or {}).get("result") or []
    detail = ""
    ok = len(locs) > 0
    if ok:
        l0 = locs[0]
        detail = f"-> {os.path.basename(l0['uri'])}:{l0['range']['start']['line']}"
        if want_line is not None:
            ok = l0["range"]["start"]["line"] == want_line
    record(f"definition {label} ({line}:{char})", ok, ms, detail)

definition(642, 5, "set_page call -> decl", want_line=33)   # set_page(c, page)
definition(22, 7, "state var use -> decl")                   # state.ticks's state? line 23? use 22
definition(1221, 9, "set_page in Actions binding", want_line=33)

print("== references ==")
msg, ms = request("textDocument/references", {
    "textDocument": {"uri": doc_uri(MAIN)},
    "position": {"line": 33, "character": 4},
    "context": {"includeDeclaration": True},
}, timeout=60)
refs = (msg or {}).get("result") or []
record("references fn set_page", len(refs) >= 3, ms, f"{len(refs)} locations")

print("== completion ==")
def completion(line, char, label, want_label=None, expect_any=True):
    msg, ms = request("textDocument/completion", {
        "textDocument": {"uri": doc_uri(MAIN)},
        "position": {"line": line, "character": char},
    })
    items = (msg or {}).get("result") or []
    if isinstance(items, dict):
        items = items.get("items", [])
    labels = {i["label"] for i in items}
    ok = (len(items) > 0) == expect_any
    if want_label:
        ok = want_label in labels
    record(f"completion {label} ({line}:{char})", ok, ms, f"{len(items)} items")
    return labels

# craft a didChange to add "  state." inside fn tick body, then complete
lines = source.split("\n")
lines.insert(24, "  state.")
edited = "\n".join(lines)
send("textDocument/didChange", {
    "textDocument": {"uri": doc_uri(MAIN), "version": 2},
    "contentChanges": [{"text": edited}],
}, notify=True)
labels = completion(24, 8, "state. members", want_label="ticks")
if labels:
    print(f"    sample: {sorted(labels)[:8]}")

# revert
send("textDocument/didChange", {
    "textDocument": {"uri": doc_uri(MAIN), "version": 3},
    "contentChanges": [{"text": source}],
}, notify=True)

print("== diagnostics on breaking edit ==")
drain_notifications()
broken = source.replace("state.ticks = state.ticks + 1", 'state.ticks = "oops"', 1)
t0 = time.monotonic()
send("textDocument/didChange", {
    "textDocument": {"uri": doc_uri(MAIN), "version": 4},
    "contentChanges": [{"text": broken}],
}, notify=True)
diags, _ = await_diagnostics(timeout=60, version=4)
ms = (time.monotonic() - t0) * 1000
n = len(diags["diagnostics"]) if diags else -1
record("diagnostics after type-error edit", diags is not None and n > 0, ms, f"{n} diagnostics")

t0 = time.monotonic()
send("textDocument/didChange", {
    "textDocument": {"uri": doc_uri(MAIN), "version": 5},
    "contentChanges": [{"text": source}],
}, notify=True)
diags, _ = await_diagnostics(timeout=60, version=5)
ms = (time.monotonic() - t0) * 1000
n = len(diags["diagnostics"]) if diags else -1
record("diagnostics clean after revert", diags is not None and n == 0, ms, f"{n} diagnostics")

print("== malformed input resilience ==")
proc.stdin.write(b"Content-Length: 5\r\n\r\n{oops")
proc.stdin.flush()
time.sleep(0.5)
alive = proc.poll() is None
record("survives malformed frame", alive, 0)

# server should still answer (guarded: known-fail if the pipe died)
try:
    hover(33, 4, "post-garbage hover")
except BrokenPipeError:
    record("post-garbage hover", False, 0, "pipe dead - server exited")

print("== shutdown ==")
try:
    msg, ms = request("shutdown", {})
    record("shutdown", msg is not None, ms)
    send("exit", {}, notify=True)
except BrokenPipeError:
    record("shutdown", False, 0, "pipe dead")
time.sleep(0.5)

fails = [r for r in results if r[0] == "FAIL"]
print(f"\n== SUMMARY: {len(results)-len(fails)}/{len(results)} passed ==")
for status, name, ms, detail in results:
    if status == "FAIL":
        print(f"  FAIL {name} {detail}")
if stderr_lines:
    print("\n== server stderr (first 10) ==")
    for line in stderr_lines[:10]:
        print(" ", line)
proc.kill()
sys.exit(1 if fails else 0)
