import fs from "node:fs";
import path from "node:path";

import { messageFromError } from "./ard.prelude.mjs";

export {
  JsonToDynamic,
  DecodeString,
  DecodeInt,
  DecodeFloat,
  DecodeBool,
  IsNil,
  DynamicToList,
  DynamicToMap,
  ExtractField,
  StrToDynamic,
  IntToDynamic,
  FloatToDynamic,
  BoolToDynamic,
  VoidToDynamic,
  ListToDynamic,
  MapToDynamic,
  JsonEncode,
  promiseResolve,
  promiseReject,
  promiseMap,
  promiseThen,
  promiseRescue,
  promiseInspect,
  promiseInspectError,
  promiseFinally,
  promiseAll,
  promiseRace,
  promiseDelay,
  fetchNative,
  fetchResponseUrl,
  fetchResponseStatus,
  fetchResponseHeaders,
  fetchResponseBody,
  fetchErrorMessage,
} from "./ard.prelude.mjs";

let stdinBuffer = "";
let stdinEOF = false;

function errorResult(error) {
  return { err: messageFromError(error) };
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
    return errorResult(error);
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
    return errorResult(error);
  }
}

export function FS_WriteFile(filePath, content) {
  try {
    fs.writeFileSync(filePath, content, "utf8");
    return { ok: undefined };
  } catch (error) {
    return errorResult(error);
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
    return errorResult(error);
  }
}

export function FS_ReadFile(filePath) {
  try {
    return { ok: fs.readFileSync(filePath, "utf8") };
  } catch (error) {
    return errorResult(error);
  }
}

export function FS_DeleteFile(filePath) {
  try {
    fs.rmSync(filePath);
    return { ok: undefined };
  } catch (error) {
    return errorResult(error);
  }
}

export function FS_Copy(from, to) {
  try {
    fs.copyFileSync(from, to);
    return { ok: undefined };
  } catch (error) {
    return errorResult(error);
  }
}

export function FS_Rename(from, to) {
  try {
    fs.renameSync(from, to);
    return { ok: undefined };
  } catch (error) {
    return errorResult(error);
  }
}

export function FS_Cwd() {
  try {
    return { ok: process.cwd() };
  } catch (error) {
    return errorResult(error);
  }
}

export function FS_Abs(filePath) {
  try {
    return { ok: path.resolve(filePath) };
  } catch (error) {
    return errorResult(error);
  }
}

export function FS_CreateDir(filePath) {
  try {
    fs.mkdirSync(filePath, { recursive: true });
    return { ok: undefined };
  } catch (error) {
    return errorResult(error);
  }
}

export function FS_DeleteDir(filePath) {
  try {
    fs.rmSync(filePath, { recursive: true, force: true });
    return { ok: undefined };
  } catch (error) {
    return errorResult(error);
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
    return errorResult(error);
  }
}
