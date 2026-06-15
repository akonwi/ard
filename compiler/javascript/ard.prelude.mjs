export function makeArdError(kind, moduleName, fnName, line, message, extra = {}) {
  const error = new globalThis.Error(message);
  error.ard_error = kind;
  error.module = moduleName;
  error.function = fnName;
  error.line = line;
  for (const key in extra) error[key] = extra[key];
  return error;
}

export function makeBreakSignal() {
  return { __ard_break: true };
}

const ARD_ENUM = Symbol.for("ard.enum");
const ARD_BYTES = Symbol.for("ard.bytes");

export function hasOwn(value, key) {
  return Boolean(value) && Object.prototype.hasOwnProperty.call(value, key);
}

export function isPlainObject(value) {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value) && !(value instanceof Map);
}

export function makeEnum(enumName, variantName, value) {
  return Object.freeze({ [ARD_ENUM]: true, enum: enumName, variant: variantName, value });
}

export function isArdEnum(value) {
  return Boolean(value) && typeof value === "object" && value[ARD_ENUM] === true;
}

export function isArdBytes(value) {
  return Boolean(value) && typeof value === "object" && value[ARD_BYTES] === true;
}

export function isEnumOf(value, enumName) {
  return isArdEnum(value) && value.enum === enumName;
}

export function isVoid(value) {
  return value === null || value === undefined;
}

export function ardEnumValue(value) {
  return isArdEnum(value) ? value.value : value;
}

export class Maybe {
  static some(value) {
    const maybe = new Maybe();
    maybe.value = value;
    return maybe;
  }

  static none() {
    return new Maybe();
  }

  isSome() {
    return Object.prototype.hasOwnProperty.call(this, "value");
  }

  isNone() {
    return !this.isSome();
  }

  or(fallback) {
    return this.isSome() ? this.value : fallback;
  }

  expect(message) {
    if (this.isSome()) return this.value;
    throw makeArdError("panic", "ard/maybe", "expect", 0, message);
  }

  map(fn) {
    return this.isSome() ? Maybe.some(fn(this.value)) : Maybe.none();
  }

  andThen(fn) {
    return this.isSome() ? fn(this.value) : Maybe.none();
  }
}

export function isArdMaybe(value) {
  return value instanceof Maybe;
}

export function isArdResult(value) {
  return value instanceof Result;
}

export class Result {
  static ok(value) {
    const result = new Result();
    result.ok = value;
    return result;
  }

  static err(error) {
    const result = new Result();
    result.error = error;
    return result;
  }

  isOk() {
    return Object.prototype.hasOwnProperty.call(this, "ok");
  }

  isErr() {
    return Object.prototype.hasOwnProperty.call(this, "error");
  }

  or(fallback) {
    return this.isOk() ? this.ok : fallback;
  }

  expect(message) {
    if (this.isOk()) return this.ok;
    throw makeArdError("panic", "ard/result", "expect", 0, message);
  }

  map(fn) {
    return this.isOk() ? Result.ok(fn(this.ok)) : this;
  }

  mapErr(fn) {
    return this.isErr() ? Result.err(fn(this.error)) : this;
  }

  andThen(fn) {
    return this.isOk() ? fn(this.ok) : this;
  }
}

export function formatDynamicForError(value) {
  if (value === null || value === undefined) return "null";
  if (typeof value === "string") return JSON.stringify(value);
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (isArdBytes(value)) return `[bytes with ${value.bytes.length} elements]`;
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

export function makeDecodeError(expected, found) {
  return {
    expected,
    found,
    path: [],
  };
}

export function toDynamicMap(value) {
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

export function ardBytesToBase64(bytes) {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

export function ardBase64ToBytes(value) {
  const binary = atob(String(value));
  const bytes = [];
  for (let i = 0; i < binary.length; i++) bytes.push(binary.charCodeAt(i));
  return bytes;
}

export function toJSONValue(value) {
  if (value === null || value === undefined) return null;
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return value;
  if (isArdBytes(value)) return ardBytesToBase64(value.bytes);
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

export function unwrapMaybe(value) {
  if (isArdMaybe(value)) {
    return hasOwn(value, "value") ? value.value : null;
  }
  return value;
}

export function toHeaderObject(headers) {
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

export function toRequestBody(body) {
  if (body === null || body === undefined) return undefined;
  if (typeof body === "string") return body;
  if (typeof body === "number" || typeof body === "boolean") return String(body);
  return JSON.stringify(toJSONValue(body));
}

export function messageFromError(error) {
  return error instanceof Error ? error.message : String(error);
}

export function JsonToDynamic(jsonString) {
  try {
    return { ok: JSON.parse(jsonString) };
  } catch (error) {
    return { err: `Error parsing JSON: ${messageFromError(error)}` };
  }
}

export function JsonParse(input) {
  try {
    return { ok: JSON.parse(input) };
  } catch (error) {
    return { err: messageFromError(error) };
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

export function DecodeByte(data) {
  const decoded = DecodeInt(data);
  if (Object.prototype.hasOwnProperty.call(decoded, "ok") && decoded.ok >= 0 && decoded.ok <= 255) return { ok: decoded.ok };
  return { err: makeDecodeError("Byte", formatDynamicForError(data)) };
}

export function DecodeRune(data) {
  const decoded = DecodeInt(data);
  const value = decoded.ok;
  if (
    Object.prototype.hasOwnProperty.call(decoded, "ok") &&
    value >= 0 &&
    value <= 0x10ffff &&
    !(value >= 0xd800 && value <= 0xdfff)
  ) {
    return { ok: value };
  }
  return { err: makeDecodeError("Rune", formatDynamicForError(data)) };
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

export function IntFromStr(value) {
  if (typeof value !== "string" || !/^[+-]?\d+$/.test(value.trim())) return null;
  const parsed = Number.parseInt(value, 10);
  return Number.isSafeInteger(parsed) ? parsed : null;
}

export function FloatFromStr(value) {
  if (typeof value !== "string" || value.trim() === "") return null;
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : null;
}

export function FloatFromInt(value) {
  return value;
}

export function FloatFloor(value) {
  return Math.floor(value);
}

function float64Parts(value) {
  const view = new DataView(new ArrayBuffer(8));
  view.setFloat64(0, value, false);
  const bits = view.getBigUint64(0, false);
  const negative = (bits >> 63n) === 1n;
  const exponentBits = Number((bits >> 52n) & 0x7ffn);
  const fraction = bits & ((1n << 52n) - 1n);
  if (exponentBits === 0) {
    return { negative, mantissa: fraction, exponent: -1074 };
  }
  return { negative, mantissa: (1n << 52n) | fraction, exponent: exponentBits - 1075 };
}

function fixedDecimalString(scaled, decimals) {
  let digits = scaled.toString();
  if (decimals === 0) return digits;
  if (digits.length <= decimals) digits = "0".repeat(decimals + 1 - digits.length) + digits;
  const point = digits.length - decimals;
  return digits.slice(0, point) + "." + digits.slice(point);
}

export function FloatFormat(value, decimals) {
  value = Number(value);
  decimals = Math.trunc(Number(decimals));
  if (!Number.isFinite(decimals) || decimals < 0) decimals = 0;

  if (Number.isNaN(value)) return "NaN";
  if (!Number.isFinite(value)) return value < 0 ? "-Inf" : "+Inf";

  const { negative, mantissa, exponent } = float64Parts(value);
  const sign = negative ? "-" : "";
  if (mantissa === 0n) return sign + fixedDecimalString(0n, decimals);

  let numerator = mantissa * 10n ** BigInt(decimals);
  let denominator = 1n;
  if (exponent >= 0) {
    numerator = numerator << BigInt(exponent);
  } else {
    denominator = denominator << BigInt(-exponent);
  }

  let scaled = numerator / denominator;
  const remainder = numerator % denominator;
  const twiceRemainder = remainder * 2n;
  if (twiceRemainder > denominator || (twiceRemainder === denominator && scaled % 2n === 1n)) {
    scaled += 1n;
  }

  return sign + fixedDecimalString(scaled, decimals);
}

function isValidByte(value) {
  return Number.isInteger(value) && value >= 0 && value <= 255;
}

function isValidRune(value) {
  return Number.isInteger(value) && value >= 0 && value <= 0x10ffff && !(value >= 0xd800 && value <= 0xdfff);
}

export function ByteFromInt(value) {
  return isValidByte(value) ? value : null;
}

export function RuneFromInt(value) {
  return isValidRune(value) ? value : null;
}

export function RuneFromStr(value) {
  const chars = Array.from(String(value));
  if (chars.length !== 1) return null;
  const rune = chars[0].codePointAt(0);
  return isValidRune(rune) ? rune : null;
}

export function ardRuneToString(value) {
  if (!isValidRune(value)) return "�";
  return String.fromCodePoint(value);
}

export function ardStringRunes(value) {
  return Array.from(String(value), (ch) => ch.codePointAt(0));
}

export function ardStringAt(value, index) {
  const runes = ardStringRunes(value);
  return index >= 0 && index < runes.length ? Maybe.some(runes[index]) : Maybe.none();
}

export function ardStringBytes(value) {
  return Array.from(new TextEncoder().encode(String(value)));
}

export function ardUTF8ByteLength(value) {
  return new TextEncoder().encode(String(value)).length;
}

export function StrSplit(input, delimiter) {
  input = String(input);
  delimiter = String(delimiter);
  return delimiter === "" ? Array.from(input) : input.split(delimiter);
}

export function StrFromUtf8(bytes) {
  if (!Array.isArray(bytes) || bytes.some((value) => !isValidByte(value))) {
    return { err: "invalid UTF-8 bytes" };
  }
  try {
    return { ok: new TextDecoder("utf-8", { fatal: true }).decode(new Uint8Array(bytes)) };
  } catch (error) {
    return { err: messageFromError(error) };
  }
}

export function StrFromRunes(runes) {
  if (!Array.isArray(runes) || runes.some((value) => !isValidRune(value))) {
    return { err: "invalid Unicode scalar value" };
  }
  try {
    let out = "";
    for (const rune of runes) out += String.fromCodePoint(rune);
    return { ok: out };
  } catch (error) {
    return { err: messageFromError(error) };
  }
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

export function BytesToDynamic(val) {
  return Object.freeze({ [ARD_BYTES]: true, bytes: Array.isArray(val) ? val.slice() : [] });
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
    return { err: messageFromError(error) };
  }
}

export function promiseResolve(value) {
  return Promise.resolve(value);
}

export function promiseReject(reason) {
  return Promise.reject(reason);
}

export function promiseMap(promise, withFn) {
  return promise.then((value) => withFn(value));
}

export function promiseThen(promise, withFn) {
  return promise.then((value) => withFn(value));
}

export function promiseRescue(promise, withFn) {
  return promise.catch((error) => withFn(error));
}

export function promiseInspect(promise, withFn) {
  return promise.then((value) => {
    withFn(value);
    return value;
  });
}

export function promiseInspectError(promise, withFn) {
  return promise.catch((error) => {
    withFn(error);
    throw error;
  });
}

export function promiseFinally(promise, withFn) {
  return promise.finally(() => {
    withFn();
  });
}

export function promiseAll(promises) {
  return Promise.all(promises);
}

export function promiseRace(promises) {
  return Promise.race(promises);
}

export function promiseDelay(ms, value) {
  return new Promise((resolve) => {
    setTimeout(() => resolve(value), ms);
  });
}

export async function fetchNative(method, url, body, headers, timeout) {
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
      url: response.url,
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

export function fetchResponseUrl(response) {
  if (!Boolean(response) || typeof response !== "object") return "";
  return typeof response.url === "string" ? response.url : String(response.url ?? "");
}

export function fetchResponseStatus(response) {
  if (!Boolean(response) || typeof response !== "object") return 0;
  return typeof response.status === "number" ? response.status : 0;
}

export function fetchResponseHeaders(response) {
  if (!Boolean(response) || typeof response !== "object") return new Map();
  if (response.headers instanceof Map) return new Map(response.headers);
  return new Map(Object.entries(response.headers ?? {}));
}

export function fetchResponseBody(response) {
  if (!Boolean(response) || typeof response !== "object") return "";
  return typeof response.body === "string" ? response.body : String(response.body ?? "");
}

export function fetchErrorMessage(reason) {
  return messageFromError(reason);
}

export function ardEq(left, right) {
  if (isArdMaybe(left) || isArdMaybe(right)) {
    if (!isArdMaybe(left) || !isArdMaybe(right)) return false;
    if (left.isNone() && right.isNone()) return true;
    if (left.isNone() || right.isNone()) return false;
    return ardEq(left.value, right.value);
  }
  return ardEnumValue(left) === ardEnumValue(right);
}

export function ardToString(value) {
  if (typeof value === "string") return value;
  if (typeof value === "number") return String(ardEnumValue(value));
  if (typeof value === "boolean") return String(value);
  if (isArdEnum(value)) return String(value.value);
  if (value && typeof value.to_str === "function") return value.to_str();
  return String(value);
}
