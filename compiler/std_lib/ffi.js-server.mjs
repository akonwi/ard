import fs from "node:fs";
import path from "node:path";

const ARD_ENUM = Symbol.for("ard.enum");

let stdinBuffer = "";
let stdinEOF = false;

function hasOwn(value, key) {
  return !!value && Object.prototype.hasOwnProperty.call(value, key);
}

function isPlainObject(value) {
  return !!value && typeof value === "object" && !Array.isArray(value) && !(value instanceof Map);
}

function isArdEnum(value) {
  return !!(value && typeof value === "object" && value[ARD_ENUM] === true);
}

function isArdMaybe(value) {
  return !!(value && typeof value === "object" && value.constructor && value.constructor.name === "Maybe");
}

function isArdResult(value) {
  return !!(value && typeof value === "object" && value.constructor && value.constructor.name === "Result");
}

function formatDynamicForError(value) {
  if (value === null || value === undefined) return "null";
  if (typeof value === "string") return JSON.stringify(value);
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (Array.isArray(value)) {
    if (value.length === 0) return "[]";
    if (value.length <= 3) return `[${value.map((item) => formatDynamicForError(item)).join(", ")}]`;
    return `[array with ${value.length} elements]`;
  }
  if (value instanceof Map) {
    const entries = Array.from(value.entries());
    if (entries.length === 0) return "{}";
    if (entries.length <= 3) {
      return `{${entries.map(([key, item]) => `${String(key)}: ${formatDynamicForError(item)}`).join(", ")}}`;
    }
    return `{object with ${entries.length} fields}`;
  }
  if (isPlainObject(value) || isArdEnum(value) || isArdMaybe(value) || isArdResult(value)) {
    const entries = Object.entries(value).filter(([key]) => !key.startsWith("__"));
    if (entries.length === 0) return "{}";
    if (entries.length <= 3) {
      return `{${entries.map(([key, item]) => `${key}: ${formatDynamicForError(item)}`).join(", ")}}`;
    }
    return `{object with ${entries.length} fields}`;
  }
  return String(value);
}

function makeDecodeError(expected, found) {
  return {
    expected,
    found,
    path: [],
  };
}

function fillStdinBuffer() {
  const chunk = Buffer.alloc(4096);
  const bytesRead = fs.readSync(0, chunk, 0, chunk.length, null);
  if (bytesRead === 0) {
    stdinEOF = true;
    return;
  }
  stdinBuffer += chunk.toString("utf8", 0, bytesRead);
}

function toDynamicMap(value) {
  if (value instanceof Map) {
    const out = {};
    for (const [key, item] of value.entries()) {
      out[String(key)] = item;
    }
    return out;
  }
  if (isPlainObject(value)) {
    return { ...value };
  }
  return {};
}

function toJSONValue(value) {
  if (value === null || value === undefined) return null;
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return value;
  if (isArdEnum(value)) return value.value;
  if (isArdMaybe(value)) {
    return hasOwn(value, "value") ? toJSONValue(value.value) : null;
  }
  if (isArdResult(value)) {
    if (hasOwn(value, "ok")) return toJSONValue(value.ok);
    if (hasOwn(value, "error")) return toJSONValue(value.error);
    if (hasOwn(value, "err")) return toJSONValue(value.err);
    return null;
  }
  if (Array.isArray(value)) return value.map((item) => toJSONValue(item));
  if (value instanceof Map) {
    const out = {};
    for (const [key, item] of value.entries()) {
      out[String(key)] = toJSONValue(item);
    }
    return out;
  }
  if (typeof value === "object") {
    const out = {};
    for (const [key, item] of Object.entries(value)) {
      out[key] = toJSONValue(item);
    }
    return out;
  }
  return value;
}

export function printLine(value) {
  process.stdout.write(String(value) + "\n");
}

export function readLine() {
  try {
    while (true) {
      const newlineIndex = stdinBuffer.indexOf("\n");
      if (newlineIndex >= 0) {
        let line = stdinBuffer.slice(0, newlineIndex);
        stdinBuffer = stdinBuffer.slice(newlineIndex + 1);
        if (line.endsWith("\r")) {
          line = line.slice(0, -1);
        }
        return { ok: line };
      }

      if (stdinEOF) {
        let line = stdinBuffer;
        stdinBuffer = "";
        if (line.endsWith("\r")) {
          line = line.slice(0, -1);
        }
        return { ok: line };
      }

      fillStdinBuffer();
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function JsonToDynamic(jsonString) {
  try {
    return { ok: JSON.parse(jsonString) };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: `Error parsing JSON: ${message}` };
  }
}

export function DecodeString(data) {
  if (data === null || data === undefined) return { err: makeDecodeError("Str", "null") };
  if (typeof data === "string") return { ok: data };
  return { err: makeDecodeError("Str", formatDynamicForError(data)) };
}

export function DecodeInt(data) {
  if (data === null || data === undefined) return { err: makeDecodeError("Int", "null") };
  if (typeof data === "number" && Number.isInteger(data)) return { ok: data };
  return { err: makeDecodeError("Int", formatDynamicForError(data)) };
}

export function DecodeFloat(data) {
  if (data === null || data === undefined) return { err: makeDecodeError("Float", "null") };
  if (typeof data === "number") return { ok: data };
  return { err: makeDecodeError("Float", formatDynamicForError(data)) };
}

export function DecodeBool(data) {
  if (data === null || data === undefined) return { err: makeDecodeError("Bool", "null") };
  if (typeof data === "boolean") return { ok: data };
  return { err: makeDecodeError("Bool", formatDynamicForError(data)) };
}

export function IsNil(data) {
  return data === null || data === undefined;
}

export function DynamicToList(data) {
  if (data === null || data === undefined) return { err: "null" };
  if (Array.isArray(data)) return { ok: data.slice() };
  return { err: formatDynamicForError(data) };
}

export function DynamicToMap(data) {
  if (data === null || data === undefined) return { err: "null" };
  if (data instanceof Map) return { ok: new Map(data) };
  if (isPlainObject(data)) return { ok: new Map(Object.entries(data)) };
  return { err: formatDynamicForError(data) };
}

export function ExtractField(data, name) {
  if (data === null || data === undefined) return { err: "null" };
  if (data instanceof Map) return { ok: data.has(name) ? data.get(name) : null };
  if (!isPlainObject(data)) return { err: formatDynamicForError(data) };
  return { ok: hasOwn(data, name) ? data[name] : null };
}

export function StrToDynamic(val) {
  return val;
}

export function IntToDynamic(val) {
  return val;
}

export function FloatToDynamic(val) {
  return val;
}

export function BoolToDynamic(val) {
  return val;
}

export function VoidToDynamic() {
  return null;
}

export function ListToDynamic(list) {
  return Array.isArray(list) ? list.slice() : [];
}

export function MapToDynamic(from) {
  return toDynamicMap(from);
}

export function JsonEncode(value) {
  try {
    return { ok: JSON.stringify(toJSONValue(value)) };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_Exists(filePath) {
  return fs.existsSync(filePath);
}

export function FS_IsFile(filePath) {
  try {
    return fs.statSync(filePath).isFile();
  } catch {
    return false;
  }
}

export function FS_IsDir(filePath) {
  try {
    return fs.statSync(filePath).isDirectory();
  } catch {
    return false;
  }
}

export function FS_CreateFile(filePath) {
  try {
    const handle = fs.openSync(filePath, "w");
    fs.closeSync(handle);
    return { ok: true };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_WriteFile(filePath, content) {
  try {
    fs.writeFileSync(filePath, content, "utf8");
    return { ok: undefined };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_AppendFile(filePath, content) {
  try {
    const handle = fs.openSync(filePath, fs.constants.O_APPEND | fs.constants.O_WRONLY);
    try {
      fs.writeFileSync(handle, content, "utf8");
    } finally {
      fs.closeSync(handle);
    }
    return { ok: undefined };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_ReadFile(filePath) {
  try {
    return { ok: fs.readFileSync(filePath, "utf8") };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_DeleteFile(filePath) {
  try {
    fs.rmSync(filePath);
    return { ok: undefined };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_Copy(from, to) {
  try {
    fs.copyFileSync(from, to);
    return { ok: undefined };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_Rename(from, to) {
  try {
    fs.renameSync(from, to);
    return { ok: undefined };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_Cwd() {
  try {
    return { ok: process.cwd() };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_Abs(filePath) {
  try {
    return { ok: path.resolve(filePath) };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_CreateDir(filePath) {
  try {
    fs.mkdirSync(filePath, { recursive: true });
    return { ok: undefined };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_DeleteDir(filePath) {
  try {
    fs.rmSync(filePath, { recursive: true, force: true });
    return { ok: undefined };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}

export function FS_ListDir(filePath) {
  try {
    const entries = fs.readdirSync(filePath, { withFileTypes: true });
    const out = {};
    for (const entry of entries) {
      out[entry.name] = entry.isFile();
    }
    return { ok: out };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { err: message };
  }
}
