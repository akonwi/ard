import { afterEach, beforeEach, expect, test } from "bun:test";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { KitError, ErrorCode } from "./kit-plugin.ts";
import { startTestPlugin, type TestKit } from "./kit-plugin-testing.ts";
import { setup } from "./plugin.ts";

// Experiments run with a system PATH so nothing optional, such as a glimpse
// dashboard, starts; every file goes to a temporary working directory.
process.env.PATH = "/usr/bin:/bin";
let cwd = "";
let kit: TestKit;
let busy = false;

beforeEach(async () => {
  cwd = realpathSync(mkdtempSync(path.join(tmpdir(), "autoresearch-test-")));
  busy = false;
  kit = await startTestPlugin(setup, {
    pluginId: "autoresearch",
    project: { cwd, git: null },
    respond: (method) => {
      if (method === "kit/session/submit-message" && busy) {
        throw new KitError(ErrorCode.TryLater, "Session is busy", { reason: "session_busy" });
      }
      return undefined;
    },
  });
});

afterEach(async () => {
  await kit.shutdown();
});

const text = (result: { content: Array<{ type: string; text?: string }> }) =>
  result.content.map((part) => part.text ?? "").join("");

test("registers its command and tools", () => {
  expect(kit.commands.get("autoresearch")).toEqual({
    id: "autoresearch",
    description: "Start, stop, clear, or resume autoresearch mode",
    argName: "off|clear|goal",
  });
  expect([...kit.tools.keys()]).toEqual(["init_experiment", "run_experiment", "log_experiment"]);
  expect(kit.footer.size).toBe(0);
});

test("a goal starts the loop, experiments are logged, and completed turns resume it", async () => {
  await kit.runCommand("autoresearch", "make it faster");
  expect(kit.messages[0]?.text).toStartWith("Start autoresearch: make it faster");
  expect(kit.toasts.map((toast) => toast.title)).toContain("Autoresearch mode ON");
  expect(kit.footer.get("status")?.[0]).toEqual({ text: "🔬 autoresearch", style: { fg: "toolText", bold: true } });

  expect(text(await kit.callTool("init_experiment", { name: "Speed", metric_name: "total_ms" }))).toStartWith(
    '✅ Experiment initialized: "Speed"',
  );
  expect(text(await kit.callTool("run_experiment", { command: "echo METRIC total_ms=12" }))).toContain("total_ms");
  expect(
    text(await kit.callTool("log_experiment", { commit: "abc123", metric: 12, status: "keep", description: "Baseline" })),
  ).toContain("Baseline");

  const lines = readFileSync(path.join(cwd, "autoresearch.jsonl"), "utf8").trim().split("\n").map((line) => JSON.parse(line));
  expect(lines[0]).toEqual({ type: "config", name: "Speed", metricName: "total_ms", metricUnit: "", bestDirection: "lower" });
  expect(lines[1]).toMatchObject({ commit: "abc123", metric: 12, status: "keep", description: "Baseline" });
  expect(kit.footer.get("status")?.map((segment) => segment.text).join("")).toContain("1 runs 1 kept");

  await kit.emit("turn.completed", { sessionId: "session-test", turn: { id: "turn_1", messages: [] } });
  expect(kit.messages.at(-1)).toMatchObject({
    text: expect.stringContaining("Run the next autoresearch iteration now."),
    idempotencyKey: "autoresearch-resume-turn_1",
  });
});

test("a busy session queues the kickoff until the running turn completes", async () => {
  busy = true;
  await kit.runCommand("autoresearch", "make it faster");
  expect(kit.messages).toEqual([]);
  expect(kit.toasts.map((toast) => toast.title)).toContain("Autoresearch queued");

  busy = false;
  await kit.emit("turn.completed", { sessionId: "session-test", turn: { id: "turn_9", messages: [] } });
  expect(kit.messages).toEqual([
    { text: expect.stringMatching(/^Start autoresearch: make it faster/), idempotencyKey: "autoresearch-kickoff-turn_9" },
  ]);
});

test("clear deletes the log and the footer item", async () => {
  await kit.runCommand("autoresearch", "make it faster");
  await kit.callTool("init_experiment", { name: "Speed", metric_name: "total_ms" });
  expect(existsSync(path.join(cwd, "autoresearch.jsonl"))).toBe(true);
  await kit.runCommand("autoresearch", "clear");
  await kit.settle();
  expect(existsSync(path.join(cwd, "autoresearch.jsonl"))).toBe(false);
  expect(kit.footer.size).toBe(0);
});

const completeTurn = (id: string) => kit.emit("turn.completed", { sessionId: "session-test", turn: { id, messages: [] } });
const resumes = () => kit.messages.filter((message) => message.text.includes("Run the next autoresearch iteration now.")).length;

async function startThenStop() {
  await kit.runCommand("autoresearch", "make it faster");
  await kit.callTool("init_experiment", { name: "Speed", metric_name: "total_ms" });
  await kit.callTool("log_experiment", { commit: "a", metric: 12, status: "keep", description: "Baseline" });
  await kit.runCommand("autoresearch", "off");
}

test("after off, a rename or cwd change does not restart the loop", async () => {
  await startThenStop();
  // The turn that was running when the user stopped it logs one more experiment.
  await kit.callTool("log_experiment", { commit: "b", metric: 11, status: "keep", description: "In flight" });
  await completeTurn("turn_1");
  await kit.emit("session.changed", { id: "session-test", name: "Renamed" });
  await kit.emit("project.changed", { cwd, git: null });
  await completeTurn("turn_2");
  expect(resumes()).toBe(0);
});

test("after off, init_experiment does not restart the loop and says so", async () => {
  await startThenStop();
  const result = text(await kit.callTool("init_experiment", { name: "Speed", metric_name: "total_ms" }));
  expect(result).toContain("Autoresearch is off: the user stopped it.");
  await kit.callTool("log_experiment", { commit: "b", metric: 11, status: "keep", description: "Again" });
  await completeTurn("turn_1");
  expect(resumes()).toBe(0);
});

test("the goal command restarts the loop after off", async () => {
  await startThenStop();
  await kit.runCommand("autoresearch", "make it faster still");
  await kit.callTool("log_experiment", { commit: "b", metric: 11, status: "keep", description: "Resumed" });
  await completeTurn("turn_1");
  expect(resumes()).toBe(1);
});

test("init_experiment starts the loop without the command, as the skill does", async () => {
  await kit.callTool("init_experiment", { name: "Speed", metric_name: "total_ms" });
  await kit.callTool("log_experiment", { commit: "a", metric: 12, status: "keep", description: "Baseline" });
  await completeTurn("turn_1");
  expect(resumes()).toBe(1);
});

test("off while the goal command waits for its before hook cancels the kickoff", async () => {
  // A before hook that runs until the test removes its gate file.
  const gate = path.join(cwd, "hook-gate");
  writeFileSync(gate, "");
  mkdirSync(path.join(cwd, "autoresearch.hooks"));
  const hook = path.join(cwd, "autoresearch.hooks", "before.sh");
  writeFileSync(hook, `#!/bin/bash\nwhile [ -e ${JSON.stringify(gate)} ]; do sleep 0.02; done\n`);
  chmodSync(hook, 0o755);

  const starting = kit.runCommand("autoresearch", "make it faster");
  await new Promise((resolve) => setTimeout(resolve, 100));
  await kit.runCommand("autoresearch", "off");
  rmSync(gate);
  await starting;
  await completeTurn("turn_1");
  expect(kit.messages).toEqual([]);
  expect(kit.toasts.map((toast) => toast.title)).not.toContain("Autoresearch mode ON");
});

