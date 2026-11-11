import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import * as fs from "node:fs";
import { createWriteStream } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";
import { type FooterSegment, isTryLater, type Kit, runPlugin, t, type ToolResult } from "./kit-plugin.ts";

const EXPERIMENT_MAX_LINES = 10;
const EXPERIMENT_MAX_BYTES = 4 * 1024;
const CHECKS_MAX_LINES = 80;
const HOOK_TIMEOUT_MS = 30_000;
const HOOK_STDOUT_MAX_BYTES = 8 * 1024;
const BENCHMARK_GUARDRAIL =
	"Be careful not to overfit to the benchmarks and do not cheat on the benchmarks.";
const METRIC_LINE_PREFIX = "METRIC";
const DENIED_METRIC_NAMES = new Set(["__proto__", "constructor", "prototype"]);
const GLIMPSE_UPDATE_INTERVAL_MS = 750;
const GLIMPSE_WIDTH = 680;
const GLIMPSE_HEIGHT = 760;
const GLIMPSE_LIVE_TAIL_LINES = 120;
const GLIMPSE_TABLE_MAX_ROWS = 40;

const RunParams = t.object({
	command: t.string({
		description:
			"Shell command to run as the experiment, e.g. './autoresearch.sh' or 'pnpm test'.",
	}),
	timeout_seconds: t.optional(
		t.number({ description: "Kill after this many seconds. Default: 600." }),
	),
	checks_timeout_seconds: t.optional(
		t.number({
			description:
				"Kill autoresearch.checks.sh after this many seconds. Default: 300.",
		}),
	),
});

const InitParams = t.object({
	name: t.string({ description: "Human-readable experiment session name." }),
	metric_name: t.string({
		description: "Primary metric name, e.g. total_ms, bundle_kb, val_bpb.",
	}),
	metric_unit: t.optional(
		t.string({ description: "Metric unit, e.g. µs, ms, s, kb, mb, or ''." }),
	),
	direction: t.optional(
		t.enum(["lower", "higher"], {
			description: "Whether lower or higher metric values are better. Default: lower.",
		}),
	),
});

const LogParams = t.object({
	commit: t.string({ description: "Git commit hash, usually short." }),
	metric: t.number({ description: "Primary optimization metric value. 0 for crashes." }),
	status: t.enum(["keep", "discard", "crash", "checks_failed"]),
	description: t.string({ description: "Short description of what this experiment tried." }),
	metrics: t.optional(
		t.raw<Record<string, number>>({
			type: "object",
			additionalProperties: true,
			description: "Secondary metrics as name/value pairs.",
		}),
	),
	force: t.optional(
		t.boolean({ description: "Allow adding newly discovered secondary metrics." }),
	),
	asi: t.optional(
		t.raw<Record<string, unknown>>({
			type: "object",
			additionalProperties: true,
			description:
				"Actionable Side Information: free-form structured diagnostics for future iterations.",
		}),
	),
});

type Status = "keep" | "discard" | "crash" | "checks_failed";
type Direction = "lower" | "higher";

type ASI = Record<string, unknown>;

type MetricDef = {
	name: string;
	unit: string;
};

type ExperimentResult = {
	commit: string;
	metric: number;
	metrics: Record<string, number>;
	status: Status;
	description: string;
	timestamp: number;
	segment: number;
	confidence: number | null;
	asi?: ASI;
};

type ExperimentState = {
	results: ExperimentResult[];
	bestMetric: number | null;
	bestDirection: Direction;
	metricName: string;
	metricUnit: string;
	secondaryMetrics: MetricDef[];
	name: string | null;
	currentSegment: number;
	maxExperiments: number | null;
	confidence: number | null;
};

type GlimpseState = {
	windowName: string | null;
	windowOpen: boolean;
	lastUpdateAt: number;
	busy: boolean;
};

type GlimpseView = {
	live?: { command: string; output: string; startedAt: number };
	final?: boolean;
};

type AutoresearchRuntime = {
	autoresearchMode: boolean;
	experimentsThisSession: number;
	lastRunChecks: { pass: boolean; output: string; duration: number } | null;
	lastRunDuration: number | null;
	runningExperiment: { startedAt: number; command: string } | null;
	state: ExperimentState;
	/** A kickoff rejected while the session was busy, submitted when the turn completes. */
	pendingKickoff: string | null;
	/** The user stopped the loop; only `/autoresearch <goal>` starts it again. */
	stoppedByUser: boolean;
	/** Bumped by every goal, off, and clear command, so a command that resumes
	 * after an await can tell that a later one superseded it. */
	activation: number;
	glimpse: GlimpseState;
};

type RunDetails = {
	command: string;
	exitCode: number | null;
	durationSeconds: number;
	passed: boolean;
	crashed: boolean;
	timedOut: boolean;
	tailOutput: string;
	checksPass: boolean | null;
	checksTimedOut: boolean;
	checksOutput: string;
	checksDuration: number;
	parsedMetrics: Record<string, number> | null;
	parsedPrimary: number | null;
	metricName: string;
	metricUnit: string;
};

type SessionSnapshot = {
	metric_name: string;
	metric_unit: string;
	direction: Direction;
	baseline_metric: number | null;
	best_metric: number | null;
	run_count: number;
	goal: string;
};

type HookPayload =
	| {
			event: "before";
			cwd: string;
			next_run: number;
			last_run: Record<string, unknown> | null;
			session: SessionSnapshot;
		}
	| {
			event: "after";
			cwd: string;
			run_entry: Record<string, unknown>;
			session: SessionSnapshot;
		};

type ProcessResult = {
	exitCode: number | null;
	timedOut: boolean;
	output: string;
	durationSeconds: number;
	tempFilePath?: string;
	totalBytes: number;
};

function createExperimentState(): ExperimentState {
	return {
		results: [],
		bestMetric: null,
		bestDirection: "lower",
		metricName: "metric",
		metricUnit: "",
		secondaryMetrics: [],
		name: null,
		currentSegment: 0,
		maxExperiments: null,
		confidence: null,
	};
}

function createRuntime(): AutoresearchRuntime {
	return {
		autoresearchMode: false,
		experimentsThisSession: 0,
		lastRunChecks: null,
		lastRunDuration: null,
		runningExperiment: null,
		state: createExperimentState(),
		pendingKickoff: null,
		stoppedByUser: false,
		activation: 0,
		glimpse: { windowName: null, windowOpen: false, lastUpdateAt: 0, busy: false },
	};
}

function parseMetricLines(output: string): Map<string, number> {
	const metrics = new Map<string, number>();
	const regex = new RegExp(`^${METRIC_LINE_PREFIX}\\s+([\\w.µ]+)=(\\S+)\\s*$`, "gm");
	let match: RegExpExecArray | null;
	while ((match = regex.exec(output)) !== null) {
		const name = match[1];
		if (!name || DENIED_METRIC_NAMES.has(name)) continue;
		const value = Number(match[2]);
		if (Number.isFinite(value)) metrics.set(name, value);
	}
	return metrics;
}

function tailText(text: string, maxLines: number, maxBytes: number): string {
	const lines = text.split("\n").slice(-maxLines).join("\n");
	const bytes = Buffer.from(lines, "utf8");
	if (bytes.length <= maxBytes) return lines;
	return "…[truncated]\n" + bytes.subarray(bytes.length - maxBytes).toString("utf8");
}

function fmtNum(n: number, decimals = 0): string {
	return n.toLocaleString(undefined, {
		maximumFractionDigits: decimals,
		minimumFractionDigits: decimals,
	});
}

function formatNum(value: number | null, unit: string): string {
	if (value === null) return "—";
	const decimals = Number.isInteger(value) ? 0 : 2;
	return `${fmtNum(value, decimals)}${unit || ""}`;
}

function isBetter(current: number, best: number, direction: Direction): boolean {
	return direction === "lower" ? current < best : current > best;
}

function sortedMedian(values: number[]): number {
	if (values.length === 0) return 0;
	const sorted = [...values].sort((a, b) => a - b);
	const mid = Math.floor(sorted.length / 2);
	return sorted.length % 2 === 0 ? (sorted[mid - 1] + sorted[mid]) / 2 : sorted[mid];
}

function currentResults(results: ExperimentResult[], segment: number): ExperimentResult[] {
	return results.filter((result) => result.segment === segment);
}

function findBaselineMetric(results: ExperimentResult[], segment: number): number | null {
	return currentResults(results, segment)[0]?.metric ?? null;
}

function findBestMetric(
	results: ExperimentResult[],
	segment: number,
	direction: Direction,
): number | null {
	const kept = currentResults(results, segment).filter(
		(result) => result.status === "keep" && result.metric > 0,
	);
	if (kept.length === 0) return findBaselineMetric(results, segment);
	return kept.reduce((best, result) => (isBetter(result.metric, best, direction) ? result.metric : best), kept[0].metric);
}

function computeConfidence(
	results: ExperimentResult[],
	segment: number,
	direction: Direction,
): number | null {
	const cur = currentResults(results, segment).filter((result) => result.metric > 0);
	if (cur.length < 3) return null;
	const baseline = cur[0]?.metric;
	if (!baseline) return null;
	const best = findBestMetric(results, segment, direction);
	if (!best || best === baseline) return null;
	const values = cur.map((result) => result.metric);
	const median = sortedMedian(values);
	const mad = sortedMedian(values.map((value) => Math.abs(value - median)));
	if (mad === 0) return null;
	return Math.abs(best - baseline) / mad;
}

function inferMetricUnit(name: string): string {
	if (name.endsWith("µs")) return "µs";
	if (name.endsWith("_ms")) return "ms";
	if (name.endsWith("_s") || name.endsWith("_sec")) return "s";
	if (name.endsWith("_kb")) return "kb";
	if (name.endsWith("_mb")) return "mb";
	return "";
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return value !== null && typeof value === "object" && !Array.isArray(value);
}

function parseJsonlEntry(line: string): Record<string, unknown> | null {
	try {
		const parsed = JSON.parse(line);
		return isRecord(parsed) ? parsed : null;
	} catch {
		return null;
	}
}

function statusFrom(value: unknown): Status {
	if (value === "discard" || value === "crash" || value === "checks_failed") return value;
	return "keep";
}

function metricsFrom(value: unknown): Record<string, number> {
	if (!isRecord(value)) return {};
	const metrics: Record<string, number> = {};
	for (const [key, metric] of Object.entries(value)) {
		if (typeof metric === "number") metrics[key] = metric;
	}
	return metrics;
}

function reconstructStateFromJsonl(content: string): ExperimentState {
	const state = createExperimentState();
	let segment = 0;
	for (const line of content.split("\n").filter(Boolean)) {
		const entry = parseJsonlEntry(line);
		if (!entry) continue;

		if (entry.type === "config") {
			if (state.results.length > 0) {
				segment += 1;
				state.secondaryMetrics = [];
			}
			state.name = typeof entry.name === "string" ? entry.name : state.name;
			state.metricName = typeof entry.metricName === "string" ? entry.metricName : state.metricName;
			state.metricUnit = typeof entry.metricUnit === "string" ? entry.metricUnit : state.metricUnit;
			state.bestDirection = entry.bestDirection === "higher" ? "higher" : "lower";
			state.currentSegment = segment;
			continue;
		}

		if (typeof entry.run !== "number") continue;
		const run: ExperimentResult = {
			commit: typeof entry.commit === "string" ? entry.commit : "",
			metric: typeof entry.metric === "number" ? entry.metric : 0,
			metrics: metricsFrom(entry.metrics),
			status: statusFrom(entry.status),
			description: typeof entry.description === "string" ? entry.description : "",
			timestamp: typeof entry.timestamp === "number" ? entry.timestamp : 0,
			segment,
			confidence: typeof entry.confidence === "number" ? entry.confidence : null,
			asi: isRecord(entry.asi) ? entry.asi : undefined,
		};
		state.results.push(run);
		for (const name of Object.keys(run.metrics)) {
			if (!state.secondaryMetrics.some((metric) => metric.name === name)) {
				state.secondaryMetrics.push({ name, unit: inferMetricUnit(name) });
			}
		}
	}
	state.bestMetric = findBaselineMetric(state.results, state.currentSegment);
	state.confidence = computeConfidence(state.results, state.currentSegment, state.bestDirection);
	return state;
}

function isAutoresearchShCommand(command: string): boolean {
	let cmd = command.trim();
	cmd = cmd.replace(/^(?:\w+=\S*\s+)+/, "");
	let previous: string;
	do {
		previous = cmd;
		cmd = cmd.replace(/^(?:env|time|nice|nohup)(?:\s+-\S+(?:\s+\d+)?)*\s+/, "");
	} while (cmd !== previous);
	return /^(?:(?:bash|sh|source)\s+(?:-\w+\s+)*)?(?:\.\/|\/[\w/.-]*\/)?autoresearch\.sh(?:\s|$)/.test(cmd);
}

function killTree(pid: number | undefined): void {
	if (!pid || pid <= 0) return;
	try {
		process.kill(-pid, "SIGTERM");
	} catch {
		try {
			process.kill(pid, "SIGTERM");
		} catch {
			// Process may have exited.
		}
	}
}

async function runProcess(
	command: string,
	args: string[],
	options: {
		cwd: string;
		timeoutMs: number;
		signal?: AbortSignal;
		onOutput?: (output: string) => void;
		stdin?: string;
	},
): Promise<ProcessResult> {
	const startedAt = Date.now();
	return new Promise((resolve) => {
		let output = "";
		let totalBytes = 0;
		let timedOut = false;
		let tempFilePath: string | undefined;
		let tempFileStream: ReturnType<typeof createWriteStream> | undefined;

		const child = spawn(command, args, {
			cwd: options.cwd,
			detached: true,
			stdio: [options.stdin === undefined ? "ignore" : "pipe", "pipe", "pipe"],
		});

		if (options.stdin !== undefined) {
			child.stdin?.write(options.stdin);
			child.stdin?.end();
		}

		const timer = setTimeout(() => {
			timedOut = true;
			killTree(child.pid);
		}, options.timeoutMs);

		const abort = () => {
			timedOut = true;
			killTree(child.pid);
		};
		options.signal?.addEventListener("abort", abort, { once: true });

		function append(chunk: Buffer): void {
			const text = chunk.toString("utf8");
			output += text;
			totalBytes += chunk.length;
			if (totalBytes > EXPERIMENT_MAX_BYTES) {
				if (!tempFilePath) {
					tempFilePath = path.join(tmpdir(), `kit-autoresearch-${randomBytes(8).toString("hex")}.log`);
					tempFileStream = createWriteStream(tempFilePath);
					tempFileStream.write(output);
				} else {
					tempFileStream?.write(chunk);
				}
			}
			options.onOutput?.(output);
		}

		child.stdout?.on("data", append);
		child.stderr?.on("data", append);
		child.on("error", (error) => append(Buffer.from(`\n[process error] ${error.message}\n`)));
		child.on("close", (code) => {
			clearTimeout(timer);
			options.signal?.removeEventListener("abort", abort);
			tempFileStream?.end();
			resolve({
				exitCode: code,
				timedOut,
				output,
				durationSeconds: (Date.now() - startedAt) / 1000,
				tempFilePath,
				totalBytes,
			});
		});
	});
}

async function exec(command: string, args: string[], cwd: string, timeoutMs = 10_000): Promise<ProcessResult> {
	return runProcess(command, args, { cwd, timeoutMs });
}

function autoresearchJsonlPath(workDir: string): string {
	return path.join(workDir, "autoresearch.jsonl");
}

function autoresearchMdPath(workDir: string): string {
	return path.join(workDir, "autoresearch.md");
}

function autoresearchScriptPath(workDir: string): string {
	return path.join(workDir, "autoresearch.sh");
}

function autoresearchChecksPath(workDir: string): string {
	return path.join(workDir, "autoresearch.checks.sh");
}

function readConfig(cwd: string): { workingDir?: string; maxIterations?: number } {
	const configPath = path.join(cwd, "autoresearch.config.json");
	if (!fs.existsSync(configPath)) return {};
	try {
		const parsed = JSON.parse(fs.readFileSync(configPath, "utf8"));
		return isRecord(parsed) ? parsed : {};
	} catch {
		return {};
	}
}

function resolveWorkDir(cwd: string): string {
	const workingDir = readConfig(cwd).workingDir;
	if (typeof workingDir !== "string" || workingDir.trim() === "") return cwd;
	return path.resolve(cwd, workingDir);
}

function readMaxExperiments(cwd: string): number | null {
	const maxIterations = readConfig(cwd).maxIterations;
	return typeof maxIterations === "number" && Number.isFinite(maxIterations) && maxIterations > 0
		? Math.floor(maxIterations)
		: null;
}

function validateWorkDir(cwd: string): string | null {
	const workDir = resolveWorkDir(cwd);
	if (!fs.existsSync(workDir)) return `Working directory does not exist: ${workDir}`;
	if (!fs.statSync(workDir).isDirectory()) return `Working directory is not a directory: ${workDir}`;
	return null;
}

function readLastRun(workDir: string): Record<string, unknown> | null {
	const jsonlPath = autoresearchJsonlPath(workDir);
	if (!fs.existsSync(jsonlPath)) return null;
	const lines = fs.readFileSync(jsonlPath, "utf8").split("\n").filter(Boolean);
	for (let index = lines.length - 1; index >= 0; index--) {
		const entry = parseJsonlEntry(lines[index]);
		if (entry && typeof entry.run === "number") return entry;
	}
	return null;
}

function buildSessionSnapshot(state: ExperimentState): SessionSnapshot {
	return {
		metric_name: state.metricName,
		metric_unit: state.metricUnit,
		direction: state.bestDirection,
		baseline_metric: state.bestMetric,
		best_metric: findBestMetric(state.results, state.currentSegment, state.bestDirection),
		run_count: state.results.length,
		goal: state.name ?? "",
	};
}

function hookScriptPath(workDir: string, stage: "before" | "after"): string {
	return path.join(workDir, "autoresearch.hooks", `${stage}.sh`);
}

function isExecutableFile(filePath: string): boolean {
	try {
		fs.accessSync(filePath, fs.constants.X_OK);
		return fs.statSync(filePath).isFile();
	} catch {
		return false;
	}
}

async function runHook(payload: HookPayload): Promise<string | null> {
	const script = hookScriptPath(payload.cwd, payload.event);
	if (!isExecutableFile(script)) return null;
	const result = await runProcess("bash", [script], {
		cwd: payload.cwd,
		timeoutMs: HOOK_TIMEOUT_MS,
		stdin: JSON.stringify(payload),
	});
	appendHookLogEntry(payload.cwd, payload.event, result);
	const output = result.output.trim();
	if (result.timedOut) return `[${payload.event} hook timed out after ${HOOK_TIMEOUT_MS / 1000}s]`;
	if (result.exitCode !== 0) return `[${payload.event} hook exited ${result.exitCode}]\n${tailText(output, 40, HOOK_STDOUT_MAX_BYTES)}`.trim();
	return output ? tailText(output, 80, HOOK_STDOUT_MAX_BYTES).trim() : null;
}

function appendHookLogEntry(workDir: string, stage: "before" | "after", result: ProcessResult): void {
	const jsonlPath = autoresearchJsonlPath(workDir);
	if (!fs.existsSync(jsonlPath)) return;
	try {
		fs.appendFileSync(
			jsonlPath,
			JSON.stringify({
				type: "hook",
				stage,
				exit_code: result.exitCode,
				duration_ms: Math.round(result.durationSeconds * 1000),
				stdout_bytes: Buffer.byteLength(result.output, "utf8"),
				timed_out: result.timedOut,
			}) + "\n",
		);
	} catch {
		// Hook observability should never break the loop.
	}
}

function cloneState(state: ExperimentState): ExperimentState {
	return {
		...state,
		results: state.results.map((result) => ({ ...result, metrics: { ...result.metrics } })),
		secondaryMetrics: state.secondaryMetrics.map((metric) => ({ ...metric })),
	};
}

/** Logs a footer change Kit rejected; the footer is decoration, not state. */
function footerFailed(error: unknown): void {
	console.error("Autoresearch footer update failed:", error);
}

function updateFooter(kit: Kit, runtime: AutoresearchRuntime): void {
	const state = runtime.state;
	if (!runtime.autoresearchMode && state.results.length === 0 && !runtime.runningExperiment) {
		kit.footer.clear("status").catch(footerFailed);
		return;
	}
	const runs = currentResults(state.results, state.currentSegment);
	const kept = runs.filter((run) => run.status === "keep").length;
	const best = findBestMetric(state.results, state.currentSegment, state.bestDirection);
	const content: FooterSegment[] = [
		{ text: "🔬 autoresearch", style: { fg: "toolText", bold: true } },
	];
	if (runtime.runningExperiment) {
		content.push({ text: " running…", style: { fg: "warningText" } });
	}
	content.push(
		{ text: ` ${runs.length}`, style: { fg: "textPrimary" } },
		{ text: " runs ", style: { fg: "textMuted" } },
		{ text: String(kept), style: { fg: "toolText" } },
		{ text: " kept", style: { fg: "textMuted" } },
	);
	if (best !== null) {
		content.push(
			{ text: " │ best ", style: { fg: "textMuted" } },
			{ text: `${state.metricName}: ${formatNum(best, state.metricUnit)}`, style: { fg: "toolText", bold: true } },
		);
	}
	if (state.confidence !== null) {
		const confidenceColor =
			state.confidence >= 2
				? "progressNormal"
				: state.confidence >= 1
					? "warningText"
					: "errorText";
		content.push(
			{ text: " │ conf ", style: { fg: "textMuted" } },
			{ text: `${state.confidence.toFixed(1)}×`, style: { fg: confidenceColor, bold: true } },
		);
	}
	kit.footer.set("status", content).catch(footerFailed);
}

function autoresearchHelp(): string {
	return [
		"Usage: /autoresearch [off|clear|<text>]",
		"",
		"<text> enters autoresearch mode and starts or resumes the loop.",
		"off leaves autoresearch mode.",
		"clear deletes autoresearch.jsonl and turns autoresearch mode off.",
		"",
		"Examples:",
		"  /autoresearch optimize unit test runtime, monitor correctness",
		"  /autoresearch model training, run train.py and optimize loss ratio",
	].join("\n");
}

// ── Glimpse dashboard ───────────────────────────────────────────────
// Optional native-window dashboard via glimpse-cli
// (https://github.com/bjesuiter/glimpse-cli). Degrades to a no-op when
// the `glimpse` binary is not installed.

let glimpseAvailability: Promise<boolean> | null = null;

function glimpseAvailable(cwd: string): Promise<boolean> {
	if (!glimpseAvailability) {
		glimpseAvailability = exec("bash", ["-c", "command -v glimpse"], cwd, 5000)
			.then((result) => result.exitCode === 0 && result.output.trim().length > 0)
			.catch(() => false);
	}
	return glimpseAvailability;
}

/**
 * Runs glimpse through bash so a missing binary yields a clean non-zero
 * exit instead of a spawn error that would leave runProcess hanging.
 */
async function runGlimpse(
	args: string[],
	cwd: string,
	stdin?: string,
	timeoutMs = 15_000,
): Promise<ProcessResult> {
	return runProcess("bash", ["-c", 'exec glimpse "$@"', "glimpse", ...args], {
		cwd,
		timeoutMs,
		stdin,
	});
}

async function glimpseShow(
	glimpse: GlimpseState,
	html: string,
	cwd: string,
): Promise<void> {
	if (!glimpse.windowName) return;
	if (glimpse.windowOpen) {
		const set = await runGlimpse(
			["set-html", "-w", glimpse.windowName, "-"],
			cwd,
			html,
			10_000,
		);
		if (set.exitCode === 0) return;
		glimpse.windowOpen = false;
	}
	const open = await runGlimpse(
		[
			"open",
			"--name",
			glimpse.windowName,
			"--replace",
			"--width",
			String(GLIMPSE_WIDTH),
			"--height",
			String(GLIMPSE_HEIGHT),
			"-",
		],
		cwd,
		html,
	);
	glimpse.windowOpen = open.exitCode === 0;
}

async function glimpseClose(glimpse: GlimpseState, cwd: string): Promise<void> {
	if (!glimpse.windowName || !glimpse.windowOpen) return;
	glimpse.windowOpen = false;
	await runGlimpse(["close", "-w", glimpse.windowName], cwd, undefined, 5000);
}

/**
 * Renders the current dashboard into the session's glimpse window.
 * Throttled while streaming live output; `force` bypasses the throttle
 * for lifecycle moments (run start/end, log, finalize). No-op when
 * glimpse-cli is not installed.
 */
async function updateGlimpseDashboard(
	runtime: AutoresearchRuntime,
	sessionId: string,
	cwd: string,
	view: GlimpseView,
	options: { force?: boolean } = {},
): Promise<void> {
	if (!(await glimpseAvailable(cwd))) return;
	const glimpse = runtime.glimpse;
	const now = Date.now();
	if (!options.force && now - glimpse.lastUpdateAt < GLIMPSE_UPDATE_INTERVAL_MS) return;
	if (glimpse.busy) return;
	glimpse.busy = true;
	glimpse.lastUpdateAt = now;
	glimpse.windowName ??= `kit-autoresearch-${sessionId.slice(0, 8)}`;
	try {
		await glimpseShow(glimpse, renderDashboardHtml(runtime.state, view), cwd);
	} catch {
		// Dashboard failures must never break the experiment loop.
	} finally {
		glimpse.busy = false;
	}
}

function escapeHtml(text: string): string {
	return text
		.replace(/&/g, "&amp;")
		.replace(/</g, "&lt;")
		.replace(/>/g, "&gt;")
		.replace(/"/g, "&quot;");
}

const GLIMPSE_STATUS_BADGES: Record<Status, { icon: string; color: string }> = {
	keep: { icon: "✓", color: "#7ee787" },
	discard: { icon: "○", color: "#a1a1a1" },
	crash: { icon: "✗", color: "#ff6467" },
	checks_failed: { icon: "⚠", color: "#ffb86a" },
};

function deltaPct(metric: number, baseline: number | null): string {
	if (baseline === null || baseline === 0 || metric <= 0) return "";
	const pct = ((metric - baseline) / baseline) * 100;
	return `${pct > 0 ? "+" : ""}${pct.toFixed(1)}%`;
}

function renderDashboardHtml(state: ExperimentState, view: GlimpseView): string {
	const runs = currentResults(state.results, state.currentSegment);
	const baseline = findBaselineMetric(state.results, state.currentSegment);
	const best = findBestMetric(state.results, state.currentSegment, state.bestDirection);
	const kept = runs.filter((run) => run.status === "keep").length;
	const improvement = best !== null ? deltaPct(best, baseline) : "";

	const rows = runs
		.map((run, index) => ({ run, number: index + 1 }))
		.slice(-GLIMPSE_TABLE_MAX_ROWS)
		.reverse()
		.map(({ run, number }) => {
			const badge = GLIMPSE_STATUS_BADGES[run.status];
			return `<tr>
				<td class="num">${number}</td>
				<td style="color:${badge.color}">${badge.icon} ${escapeHtml(run.status)}</td>
				<td class="num">${escapeHtml(formatNum(run.metric > 0 ? run.metric : null, state.metricUnit))}</td>
				<td class="num muted">${escapeHtml(deltaPct(run.metric, baseline))}</td>
				<td class="mono muted">${escapeHtml(run.commit)}</td>
				<td class="desc">${escapeHtml(run.description)}</td>
			</tr>`;
		})
		.join("\n");

	const liveBlock = view.live
		? (() => {
				const elapsed = ((Date.now() - view.live.startedAt) / 1000).toFixed(0);
				const tail = view.live.output
					.split("\n")
					.slice(-GLIMPSE_LIVE_TAIL_LINES)
					.join("\n");
				return `<div class="section">
					<div class="live-header"><span class="pulse">●</span> run #${runs.length + 1} · ${elapsed}s · <span class="mono">${escapeHtml(view.live.command)}</span></div>
					<pre>${escapeHtml(tail) || "(no output yet)"}</pre>
				</div>`;
			})()
		: "";

	const finalBlock = view.final
		? `<div class="final">🏁 Session complete — ${runs.length} runs, ${kept} kept${
				best !== null && improvement
					? `, best ${escapeHtml(state.metricName)}: ${escapeHtml(formatNum(best, state.metricUnit))} (${escapeHtml(improvement)} vs baseline)`
					: ""
			}</div>`
		: "";

	const confidence =
		state.confidence !== null
			? `<span class="stat"><span class="muted">conf</span> ${state.confidence.toFixed(1)}×</span>`
			: "";

	return `<!doctype html>
<html><head><meta charset="utf-8"><style>
	body { background:#0a0a0a; color:#fafafa; font:12px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace; margin:0; padding:14px 16px; }
	h1 { font-size:14px; margin:0 0 2px; color:#6cb6ff; }
	.meta { color:#a1a1a1; margin-bottom:10px; }
	.stat { margin-right:12px; }
	.muted { color:#a1a1a1; }
	.mono { font-family:inherit; color:#d4d4d4; }
	.section { border:1px solid #262626; border-radius:4px; margin:10px 0; }
	.live-header { padding:6px 10px; border-bottom:1px solid #262626; color:#ffb86a; }
	.pulse { color:#ffb86a; }
	pre { margin:0; padding:8px 10px; max-height:340px; overflow-y:auto; white-space:pre-wrap; word-break:break-word; color:#d4d4d4; display:flex; flex-direction:column-reverse; }
	.final { border:1px solid #7ee787; border-radius:4px; color:#7ee787; padding:8px 10px; margin:10px 0; }
	table { border-collapse:collapse; width:100%; margin-top:8px; }
	th { text-align:left; color:#a1a1a1; font-weight:normal; border-bottom:1px solid #262626; padding:2px 8px 4px 0; }
	td { padding:3px 8px 3px 0; border-bottom:1px solid #171717; vertical-align:top; }
	td.num { text-align:right; white-space:nowrap; }
	td.desc { color:#d4d4d4; }
</style></head><body>
	<h1>🔬 ${escapeHtml(state.name ?? "autoresearch")}</h1>
	<div class="meta">
		<span class="stat">${escapeHtml(state.metricName)}${state.metricUnit ? ` (${escapeHtml(state.metricUnit)})` : ""} · ${state.bestDirection} is better</span>
		<span class="stat"><span class="muted">runs</span> ${runs.length}</span>
		<span class="stat"><span class="muted">kept</span> ${kept}</span>
		<span class="stat"><span class="muted">baseline</span> ${escapeHtml(formatNum(baseline, state.metricUnit))}</span>
		<span class="stat"><span class="muted">best</span> ${escapeHtml(formatNum(best, state.metricUnit))}${improvement ? ` <span class="muted">(${escapeHtml(improvement)})</span>` : ""}</span>
		${confidence}
	</div>
	${finalBlock}
	${liveBlock}
	<table>
		<tr><th>#</th><th>status</th><th>${escapeHtml(state.metricName)}</th><th>Δ</th><th>commit</th><th>description</th></tr>
		${rows || `<tr><td colspan="6" class="muted">no runs yet</td></tr>`}
	</table>
</body></html>`;
}

function composeResumeMessage(): string {
	return [
		"Run the next autoresearch iteration now.",
		"Use persisted autoresearch state as needed, pick the most promising hypothesis, then call autoresearch__run_experiment and autoresearch__log_experiment.",
		BENCHMARK_GUARDRAIL,
	].join(" ");
}

export async function setup(kit: Kit) {
	const runtimes = new Map<string, AutoresearchRuntime>();
	let glimpseMissingToastShown = false;

	async function maybeToastGlimpseMissing(cwd: string): Promise<void> {
		if (glimpseMissingToastShown) return;
		if (await glimpseAvailable(cwd)) return;
		glimpseMissingToastShown = true;
		kit.ui.toast({
			title: "Autoresearch dashboard unavailable",
			subtitle:
				"Install glimpse-cli (npm install -g glimpse-cli) for a live experiment window.",
			variant: "info",
		});
	}

	function runtimeForSession(sessionId = kit.session.id): AutoresearchRuntime {
		let runtime = runtimes.get(sessionId);
		if (!runtime) {
			runtime = createRuntime();
			runtimes.set(sessionId, runtime);
		}
		return runtime;
	}

	function reconstructRuntime(runtime: AutoresearchRuntime, cwd: string): void {
		const workDir = resolveWorkDir(cwd);
		runtime.state = createExperimentState();
		runtime.state.maxExperiments = readMaxExperiments(cwd);
		const jsonlPath = autoresearchJsonlPath(workDir);
		if (fs.existsSync(jsonlPath)) {
			try {
				runtime.state = reconstructStateFromJsonl(fs.readFileSync(jsonlPath, "utf8"));
				runtime.state.maxExperiments = readMaxExperiments(cwd);
			} catch {
				// Keep empty state if reconstruction fails.
			}
		}
		updateFooter(kit, runtime);
	}

	function warnSubmissionFailed(error: unknown): void {
		kit.ui.toast({
			title: "Autoresearch could not continue",
			subtitle: error instanceof Error ? error.message : String(error),
			variant: "warning",
		});
	}

	// Kit admits a message only when the session is idle. A busy rejection means
	// the user or queued input took over; the next completed turn tries again.
	function continueLoop(runtime: AutoresearchRuntime, completedTurnId: string): void {
		if (!runtime.autoresearchMode) return;
		const kickoff = runtime.pendingKickoff;
		const message = kickoff ?? composeResumeMessage();
		// Keyed by the completed turn so a repeated completion cannot resume twice.
		const idempotencyKey = `autoresearch-${kickoff ? "kickoff" : "resume"}-${completedTurnId}`;
		kit.session.submitMessage(message, { idempotencyKey }).then(
			() => {
				if (runtime.pendingKickoff === kickoff) runtime.pendingKickoff = null;
			},
			(error: unknown) => {
				if (!isTryLater(error, "session_busy")) warnSubmissionFailed(error);
			},
		);
	}

	await kit.commands.register({
		id: "autoresearch",
		description: "Start, stop, clear, or resume autoresearch mode",
		argName: "off|clear|goal",
		async execute(args) {
			const runtime = runtimeForSession(kit.session.id);
			const trimmedArgs = args.trim();
			const command = trimmedArgs.toLowerCase();

			if (!trimmedArgs) {
				kit.ui.toast({ title: "Autoresearch", subtitle: autoresearchHelp(), variant: "info" });
				return;
			}

			if (command === "off") {
				runtime.autoresearchMode = false;
				runtime.stoppedByUser = true;
				runtime.activation += 1;
				runtime.experimentsThisSession = 0;
				runtime.lastRunChecks = null;
				runtime.lastRunDuration = null;
				runtime.runningExperiment = null;
				runtime.pendingKickoff = null;
				updateFooter(kit, runtime);
				if (runtime.state.results.length > 0) {
					// Leave the window up showing the finalized report.
					void updateGlimpseDashboard(
						runtime,
						kit.session.id,
						resolveWorkDir(kit.project.cwd),
						{ final: true },
						{ force: true },
					);
				}
				kit.ui.toast({ title: "Autoresearch mode OFF", variant: "info" });
				return;
			}


			if (command === "clear") {
				const workDir = resolveWorkDir(kit.project.cwd);
				const jsonlPath = autoresearchJsonlPath(workDir);
				runtime.autoresearchMode = false;
				runtime.stoppedByUser = true;
				runtime.activation += 1;
				runtime.experimentsThisSession = 0;
				runtime.lastRunChecks = null;
				runtime.lastRunDuration = null;
				runtime.runningExperiment = null;
				runtime.state = createExperimentState();
				runtime.pendingKickoff = null;
				if (fs.existsSync(jsonlPath)) fs.unlinkSync(jsonlPath);
				updateFooter(kit, runtime);
				void glimpseClose(runtime.glimpse, workDir);
				kit.ui.toast({ title: "Autoresearch cleared", subtitle: "Deleted autoresearch.jsonl.", variant: "info" });
				return;
			}

			reconstructRuntime(runtime, kit.project.cwd);
			runtime.autoresearchMode = true;
			runtime.stoppedByUser = false;
			runtime.pendingKickoff = null;
			const activation = ++runtime.activation;
			const workDir = resolveWorkDir(kit.project.cwd);
			const rulesLoaded = fs.existsSync(autoresearchMdPath(workDir));
			const activationSteer = await runHook({
				event: "before",
				cwd: workDir,
				next_run: runtime.state.results.length + 1,
				last_run: readLastRun(workDir),
				session: buildSessionSnapshot(runtime.state),
			});
			// The hook can take up to 30 s; the user may have stopped or restarted
			// autoresearch meanwhile.
			if (runtime.activation !== activation) return;
			const kickoff = rulesLoaded
				? `Autoresearch mode active. ${trimmedArgs} ${BENCHMARK_GUARDRAIL}`
				: `Start autoresearch: ${trimmedArgs} ${BENCHMARK_GUARDRAIL}`;
			updateFooter(kit, runtime);
			void maybeToastGlimpseMissing(workDir);
			if (runtime.state.results.length > 0) {
				void updateGlimpseDashboard(runtime, kit.session.id, workDir, {}, { force: true });
			}
			kit.ui.toast({
				title: "Autoresearch mode ON",
				subtitle: rulesLoaded ? "Resuming from autoresearch.md." : "No autoresearch.md found; setting up.",
				variant: "info",
			});
			const message = activationSteer ? `${activationSteer}\n\n${kickoff}` : kickoff;
			try {
				await kit.session.submitMessage(message);
			} catch (error) {
				if (runtime.activation !== activation) return;
				if (!isTryLater(error, "session_busy")) {
					warnSubmissionFailed(error);
					return;
				}
				// The session is running a turn; start when it completes.
				runtime.pendingKickoff = message;
				kit.ui.toast({
					title: "Autoresearch queued",
					subtitle: "It will start when the current turn finishes.",
					variant: "info",
				});
			}
		},
	});

	await kit.tools.register({
		id: "init_experiment",
		label: "Init Experiment",
		description:
			"Initialize an autoresearch experiment session. Writes the config header to autoresearch.jsonl.",
		promptSnippet: "Initialize autoresearch session: name, metric, unit, direction.",
		promptGuidelines: [
			"Call autoresearch__init_experiment exactly once at the start of an autoresearch session before the first autoresearch__run_experiment.",
			"If autoresearch.jsonl already exists with a config, do not call autoresearch__init_experiment again unless changing target/metric/workload.",
		],
		input: InitParams,
		async execute(params): Promise<ToolResult> {
			const runtime = runtimeForSession();
			const cwd = kit.project.cwd;
			const error = validateWorkDir(cwd);
			if (error) return { content: [{ type: "text", text: `❌ ${error}` }], details: {} };

			const state = runtime.state;
			const isReinit = state.results.length > 0;
			state.name = params.name;
			state.metricName = params.metric_name;
			state.metricUnit = params.metric_unit ?? "";
			state.bestDirection = params.direction ?? "lower";
			if (isReinit) state.currentSegment += 1;
			state.bestMetric = null;
			state.secondaryMetrics = [];
			state.confidence = null;
			state.maxExperiments = readMaxExperiments(cwd);

			const workDir = resolveWorkDir(cwd);
			const config = JSON.stringify({
				type: "config",
				name: state.name,
				metricName: state.metricName,
				metricUnit: state.metricUnit,
				bestDirection: state.bestDirection,
			});
			fs.appendFileSync(autoresearchJsonlPath(workDir), `${config}\n`);
			// The skill starts the loop here, but not after the user stopped it.
			if (!runtime.stoppedByUser) runtime.autoresearchMode = true;
			updateFooter(kit, runtime);
			const stoppedNote = runtime.stoppedByUser
				? "\nAutoresearch is off: the user stopped it. Do not continue the loop; they can resume it with /autoresearch <goal>."
				: "";
			const limitNote = state.maxExperiments !== null ? `\nMax iterations: ${state.maxExperiments}` : "";
			const workDirNote = workDir !== cwd ? `\nWorking directory: ${workDir}` : "";
			return {
				content: [
					{
						type: "text",
						text: `✅ Experiment initialized: "${state.name}"${isReinit ? " (new segment)" : ""}\nMetric: ${state.metricName} (${state.metricUnit || "unitless"}, ${state.bestDirection} is better)${limitNote}${workDirNote}\nConfig written to autoresearch.jsonl.${stoppedNote || " Now run the baseline with autoresearch__run_experiment."}`,
					},
				],
				details: { state: cloneState(state) },
			};
		},
	});

	await kit.tools.register({
		id: "run_experiment",
		label: "Run Experiment",
		description:
			"Run a timed experiment command, capture output, parse METRIC name=value lines, and run autoresearch.checks.sh when present.",
		promptSnippet: "Run a timed autoresearch experiment command.",
		promptGuidelines: [
			"Use autoresearch__run_experiment instead of bash for benchmark/experiment commands.",
			"After autoresearch__run_experiment, always call autoresearch__log_experiment.",
			"If METRIC lines are parsed, use those exact values in autoresearch__log_experiment.",
		],
		input: RunParams,
		async execute(params, { signal }): Promise<ToolResult> {
			const runtime = runtimeForSession();
			const state = runtime.state;
			const cwd = kit.project.cwd;
			const error = validateWorkDir(cwd);
			if (error) {
				return { content: [{ type: "text", text: `❌ ${error}` }], details: {} as RunDetails };
			}
			const workDir = resolveWorkDir(cwd);
			if (state.maxExperiments !== null) {
				const segmentCount = currentResults(state.results, state.currentSegment).length;
				if (segmentCount >= state.maxExperiments) {
					return {
						content: [{ type: "text", text: `🛑 Maximum experiments reached (${state.maxExperiments}). Stop the loop or call autoresearch__init_experiment for a new segment.` }],
						details: {} as RunDetails,
					};
				}
			}

			if (fs.existsSync(autoresearchScriptPath(workDir)) && !isAutoresearchShCommand(params.command)) {
				return {
					content: [{ type: "text", text: `❌ autoresearch.sh exists; run it instead of a custom command. Use autoresearch__run_experiment({ command: "./autoresearch.sh" }).` }],
					details: {} as RunDetails,
				};
			}

			const sessionId = kit.session.id;
			const startedAt = Date.now();
			runtime.runningExperiment = { startedAt, command: params.command };
			updateFooter(kit, runtime);
			void updateGlimpseDashboard(
				runtime,
				sessionId,
				workDir,
				{ live: { command: params.command, output: "", startedAt } },
				{ force: true },
			);
			const result = await runProcess("bash", ["-c", params.command], {
				cwd: workDir,
				timeoutMs: (params.timeout_seconds ?? 600) * 1000,
				signal,
				onOutput: (output) => {
					void updateGlimpseDashboard(runtime, sessionId, workDir, {
						live: { command: params.command, output, startedAt },
					});
				},
			});

			const passed = result.exitCode === 0 && !result.timedOut;
			let checksPass: boolean | null = null;
			let checksTimedOut = false;
			let checksOutput = "";
			let checksDuration = 0;
			if (passed && fs.existsSync(autoresearchChecksPath(workDir))) {
				const checks = await runProcess("bash", [autoresearchChecksPath(workDir)], {
					cwd: workDir,
					timeoutMs: (params.checks_timeout_seconds ?? 300) * 1000,
					signal,
				});
				checksPass = checks.exitCode === 0 && !checks.timedOut;
				checksTimedOut = checks.timedOut;
				checksOutput = tailText(checks.output, CHECKS_MAX_LINES, EXPERIMENT_MAX_BYTES);
				checksDuration = checks.durationSeconds;
				runtime.lastRunChecks = { pass: checksPass, output: checksOutput, duration: checksDuration };
			} else {
				runtime.lastRunChecks = null;
			}

			runtime.lastRunDuration = result.durationSeconds;
			runtime.runningExperiment = null;
			const parsed = parseMetricLines(result.output);
			const parsedMetrics = parsed.size > 0 ? Object.fromEntries(parsed.entries()) : null;
			const parsedPrimary = parsedMetrics?.[state.metricName] ?? null;
			const tailOutput = tailText(result.output, EXPERIMENT_MAX_LINES, EXPERIMENT_MAX_BYTES);
			let text = `${passed ? "✅" : "❌"} Experiment ${passed ? "passed" : "failed"} in ${result.durationSeconds.toFixed(1)}s`;
			if (result.timedOut) text += " (timed out)";
			text += `\n\n${tailOutput || "(no output)"}`;
			if (result.tempFilePath) text += `\n\nFull output saved to: ${result.tempFilePath}`;
			if (parsedMetrics) {
				const secondary = Object.entries(parsedMetrics).filter(([name]) => name !== state.metricName);
				text += `\n\nParsed metrics: ${Object.entries(parsedMetrics).map(([name, value]) => `${name}=${value}`).join(", ")}`;
				text += `\nUse these values in autoresearch__log_experiment: metric=${parsedPrimary ?? "?"}, metrics={${secondary.map(([name, value]) => `"${name}": ${value}`).join(", ")}}`;
			}
			if (checksPass !== null) {
				text += checksPass
					? `\n\n✅ autoresearch.checks.sh passed in ${checksDuration.toFixed(1)}s`
					: `\n\n❌ autoresearch.checks.sh failed in ${checksDuration.toFixed(1)}s${checksTimedOut ? " (timed out)" : ""}\n${checksOutput}`;
			}
			updateFooter(kit, runtime);
			void updateGlimpseDashboard(
				runtime,
				sessionId,
				workDir,
				{
					live: {
						command: params.command,
						output: `${result.output}\n[${passed ? "finished" : "failed"} in ${result.durationSeconds.toFixed(1)}s · awaiting autoresearch__log_experiment]`,
						startedAt,
					},
				},
				{ force: true },
			);
			return {
				content: [{ type: "text", text }],
				details: {
					command: params.command,
					exitCode: result.exitCode,
					durationSeconds: result.durationSeconds,
					passed,
					crashed: !passed,
					timedOut: result.timedOut,
					tailOutput,
					checksPass,
					checksTimedOut,
					checksOutput,
					checksDuration,
					parsedMetrics,
					parsedPrimary,
					metricName: state.metricName,
					metricUnit: state.metricUnit,
				},
			};
		},
	});

	await kit.tools.register({
		id: "log_experiment",
		label: "Log Experiment",
		description:
			"Record an autoresearch experiment result. Keeps auto-commit; discard/crash/checks_failed auto-revert code changes while preserving autoresearch files.",
		promptSnippet: "Log autoresearch result and keep/discard/revert as needed.",
		promptGuidelines: [
			"Always call autoresearch__log_experiment after autoresearch__run_experiment.",
			"Use keep only when the primary metric improved and checks passed.",
			"Use discard for worse/unchanged results, crash for benchmark failure, and checks_failed when autoresearch.checks.sh fails.",
			"Always include asi with at least a hypothesis; on discard/crash include rollback_reason and next_action_hint.",
		],
		input: LogParams,
		async execute(params): Promise<ToolResult> {
			const runtime = runtimeForSession();
			const state = runtime.state;
			const cwd = kit.project.cwd;
			const error = validateWorkDir(cwd);
			if (error) return { content: [{ type: "text", text: `❌ ${error}` }], details: {} };
			const workDir = resolveWorkDir(cwd);
			const secondaryMetrics = params.metrics ?? {};

			if (params.status === "keep" && runtime.lastRunChecks && !runtime.lastRunChecks.pass) {
				return {
					content: [{ type: "text", text: `❌ Cannot keep: autoresearch.checks.sh failed. Log as checks_failed.\n\n${runtime.lastRunChecks.output.slice(-500)}` }],
					details: {},
				};
			}

			if (state.secondaryMetrics.length > 0) {
				const known = new Set(state.secondaryMetrics.map((metric) => metric.name));
				const provided = new Set(Object.keys(secondaryMetrics));
				const missing = [...known].filter((name) => !provided.has(name));
				if (missing.length > 0) {
					return { content: [{ type: "text", text: `❌ Missing secondary metrics: ${missing.join(", ")}` }], details: {} };
				}
				const added = [...provided].filter((name) => !known.has(name));
				if (added.length > 0 && !params.force) {
					return { content: [{ type: "text", text: `❌ New secondary metrics not previously tracked: ${added.join(", ")}. Retry with force:true only if they are worth tracking.` }], details: {} };
				}
			}

			const experiment: ExperimentResult = {
				commit: params.commit.slice(0, 7),
				metric: params.metric,
				metrics: secondaryMetrics,
				status: params.status,
				description: params.description,
				timestamp: Date.now(),
				segment: state.currentSegment,
				confidence: null,
				asi: params.asi && Object.keys(params.asi).length > 0 ? params.asi : undefined,
			};

			state.results.push(experiment);
			runtime.experimentsThisSession += 1;
			for (const name of Object.keys(secondaryMetrics)) {
				if (!state.secondaryMetrics.some((metric) => metric.name === name)) {
					state.secondaryMetrics.push({ name, unit: inferMetricUnit(name) });
				}
			}
			state.bestMetric = findBaselineMetric(state.results, state.currentSegment);
			state.confidence = computeConfidence(state.results, state.currentSegment, state.bestDirection);
			experiment.confidence = state.confidence;

			const segmentCount = currentResults(state.results, state.currentSegment).length;
			let text = `Logged #${state.results.length}: ${experiment.status} — ${experiment.description}`;
			if (state.bestMetric !== null) {
				text += `\nBaseline ${state.metricName}: ${formatNum(state.bestMetric, state.metricUnit)}`;
				if (segmentCount > 1 && params.metric > 0) {
					const pct = ((params.metric - state.bestMetric) / state.bestMetric) * 100;
					text += ` | this: ${formatNum(params.metric, state.metricUnit)} (${pct > 0 ? "+" : ""}${pct.toFixed(1)}%)`;
				}
			}
			if (state.confidence !== null) text += `\nConfidence: ${state.confidence.toFixed(1)}× noise floor`;

			if (params.status === "keep") {
				const resultData = JSON.stringify({ status: params.status, [state.metricName || "metric"]: params.metric, ...secondaryMetrics });
				const commitMsg = `${params.description}\n\nResult: ${resultData}`;
				const add = await exec("git", ["add", "-A"], workDir);
				if (add.exitCode !== 0) {
					text += `\n⚠️ git add failed: ${tailText(add.output, 8, 1000)}`;
				} else {
					const diff = await exec("git", ["diff", "--cached", "--quiet"], workDir);
					if (diff.exitCode === 0) {
						text += "\nGit: nothing to commit";
					} else {
						const commit = await exec("git", ["commit", "-m", commitMsg], workDir);
						if (commit.exitCode === 0) {
							text += `\nGit: committed — ${(commit.output.trim().split("\n")[0] ?? "").trim()}`;
							const sha = await exec("git", ["rev-parse", "--short=7", "HEAD"], workDir, 5000);
							const newSha = sha.output.trim();
							if (sha.exitCode === 0 && newSha) experiment.commit = newSha.slice(0, 7);
						} else {
							text += `\n⚠️ git commit failed: ${tailText(commit.output, 8, 1000)}`;
						}
					}
				}
			}

			const jsonlEntry: Record<string, unknown> = { run: state.results.length, ...experiment };
			if (!experiment.asi) delete jsonlEntry.asi;
			fs.appendFileSync(autoresearchJsonlPath(workDir), `${JSON.stringify(jsonlEntry)}\n`);

			if (params.status !== "keep") {
				const revertScript = "git checkout -- . ':(exclude,glob)**/autoresearch.*' ':(exclude,glob)**/autoresearch.*/**' && git clean -fd -e 'autoresearch.*' -e '**/autoresearch.*/**' 2>/dev/null";
				const revert = await exec("bash", ["-c", revertScript], workDir);
				text += revert.exitCode === 0
					? `\nGit: reverted changes (${params.status}); autoresearch files preserved`
					: `\n⚠️ git revert failed: ${tailText(revert.output, 8, 1000)}`;
			}

			const afterSteer = await runHook({
				event: "after",
				cwd: workDir,
				run_entry: jsonlEntry,
				session: buildSessionSnapshot(state),
			});
			if (afterSteer) text += `\n\nAfter-hook guidance:\n${afterSteer}`;

			const limitReached = state.maxExperiments !== null && segmentCount >= state.maxExperiments;
			if (limitReached) {
				text += `\n\n🛑 Maximum experiments reached (${state.maxExperiments}). STOP the experiment loop now.`;
				runtime.autoresearchMode = false;
			} else if (runtime.autoresearchMode) {
				const beforeSteer = await runHook({
					event: "before",
					cwd: workDir,
					next_run: state.results.length + 1,
					last_run: jsonlEntry,
					session: buildSessionSnapshot(state),
				});
				if (beforeSteer) text += `\n\nBefore-hook guidance for next iteration:\n${beforeSteer}`;
			}

			const wallClockSeconds = runtime.lastRunDuration;
			runtime.runningExperiment = null;
			runtime.lastRunChecks = null;
			runtime.lastRunDuration = null;
			updateFooter(kit, runtime);
			void updateGlimpseDashboard(
				runtime,
				kit.session.id,
				workDir,
				{ final: limitReached || runtime.stoppedByUser },
				{ force: true },
			);
			return {
				content: [{ type: "text", text }],
				details: { experiment: { ...experiment, metrics: { ...experiment.metrics } }, state: cloneState(state), wallClockSeconds },
			};
		},
	});

	// The working directory or session changed: reload persisted state.
	for (const event of ["project.changed", "session.changed"] as const) {
		kit.on(event, () => {
			const runtime = runtimeForSession(kit.session.id);
			reconstructRuntime(runtime, kit.project.cwd);
		});
	}

	kit.on("turn.completed", (event) => {
		const runtime = runtimeForSession(kit.session.id);
		runtime.runningExperiment = null;
		updateFooter(kit, runtime);
		if (runtime.pendingKickoff || runtime.experimentsThisSession > 0) continueLoop(runtime, event.turn.id);
	});

	return () => {
		for (const runtime of runtimes.values()) {
			void glimpseClose(runtime.glimpse, kit.project.cwd);
		}
		// Kit removes the plugin's footer item when it stops.
	};
}

if (import.meta.main) runPlugin(setup);
