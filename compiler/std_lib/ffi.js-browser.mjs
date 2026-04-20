const ARD_ENUM = Symbol.for("ard.enum");

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

function unwrapMaybe(value) {
  if (isArdMaybe(value)) {
    return hasOwn(value, "value") ? value.value : null;
  }
  return value;
}

function toHeaderObject(headers) {
  if (headers instanceof Map) {
    const out = {};
    for (const [key, value] of headers.entries()) {
      out[String(key)] = String(value);
    }
    return out;
  }
  if (isPlainObject(headers)) {
    const out = {};
    for (const [key, value] of Object.entries(headers)) {
      out[String(key)] = String(value);
    }
    return out;
  }
  return {};
}

function toRequestBody(body) {
  if (body === null || body === undefined) return undefined;
  if (typeof body === "string") return body;
  if (typeof body === "number" || typeof body === "boolean") return String(body);
  return JSON.stringify(toJSONValue(body));
}

function messageFromError(error) {
  return error instanceof Error ? error.message : String(error);
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

export function JSPromiseResolve(value) {
  return Promise.resolve(value);
}

export function JSPromiseReject(reason) {
  return Promise.reject(reason);
}

export function JSPromiseMap(promise, withFn) {
  return promise.then((value) => withFn(value));
}

export function JSPromiseThen(promise, withFn) {
  return promise.then((value) => withFn(value));
}

export function JSPromiseMapError(promise, withFn) {
  return promise.catch((error) => withFn(error));
}

export function JSPromiseRecover(promise, withFn) {
  return promise.catch((error) => withFn(error));
}

export function JSPromiseInspect(promise, withFn) {
  return promise.then((value) => {
    withFn(value);
    return value;
  });
}

export function JSPromiseInspectError(promise, withFn) {
  return promise.catch((error) => {
    withFn(error);
    throw error;
  });
}

export function JSPromiseFinally(promise, withFn) {
  return promise.finally(() => {
    withFn();
  });
}

export function JSPromiseAll(promises) {
  return Promise.all(promises);
}

export function JSPromiseRace(promises) {
  return Promise.race(promises);
}

export function JSPromiseDelay(ms, value) {
  return new Promise((resolve) => {
    setTimeout(() => resolve(value), ms);
  });
}

export async function JSHTTP_Fetch(method, url, body, headers, timeout) {
  const timeoutSeconds = unwrapMaybe(timeout);
  const controller = typeof AbortController === "function" ? new AbortController() : null;
  let timeoutId = null;

  try {
    if (controller && typeof timeoutSeconds === "number") {
      timeoutId = setTimeout(() => controller.abort(), timeoutSeconds * 1000);
    }

    const response = await fetch(url, {
      method,
      headers: toHeaderObject(headers),
      body: toRequestBody(body),
      signal: controller ? controller.signal : undefined,
    });

    const responseBody = await response.text();
    const responseHeaders = new Map();
    response.headers.forEach((value, key) => {
      responseHeaders.set(key, value);
    });

    return {
      status: response.status,
      headers: responseHeaders,
      body: responseBody,
    };
  } catch (error) {
    throw messageFromError(error);
  } finally {
    if (timeoutId !== null) {
      clearTimeout(timeoutId);
    }
  }
}

export function JSHTTP_ResponseStatus(response) {
  if (!response || typeof response !== "object") return 0;
  return typeof response.status === "number" ? response.status : 0;
}

export function JSHTTP_ResponseHeaders(response) {
  if (!response || typeof response !== "object") return new Map();
  if (response.headers instanceof Map) return new Map(response.headers);
  return new Map(Object.entries(response.headers ?? {}));
}

export function JSHTTP_ResponseBody(response) {
  if (!response || typeof response !== "object") return "";
  return typeof response.body === "string" ? response.body : String(response.body ?? "");
}

export function JSHTTP_ErrorMessage(reason) {
  return messageFromError(reason);
}
