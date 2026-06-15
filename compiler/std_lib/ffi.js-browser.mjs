function errorResult(error) {
  return { err: error instanceof Error ? error.message : String(error) };
}

function maybeBool(value) {
  return value && typeof value.isSome === "function" && value.isSome() ? Boolean(value.value) : false;
}

function bytesToBinary(bytes) {
  let out = "";
  for (const byte of bytes) out += String.fromCharCode(byte);
  return out;
}

function binaryToBytes(binary) {
  const bytes = [];
  for (let i = 0; i < binary.length; i++) bytes.push(binary.charCodeAt(i));
  return bytes;
}

function toURLBase64(encoded, noPad) {
  let out = encoded.replace(/\+/g, "-").replace(/\//g, "_");
  return maybeBool(noPad) ? out.replace(/=+$/g, "") : out;
}

function fromURLBase64(input, noPad) {
  let out = String(input).replace(/-/g, "+").replace(/_/g, "/");
  if (maybeBool(noPad)) out += "=".repeat((4 - (out.length % 4)) % 4);
  return out;
}

export function Base64Encode(input, noPad) {
  const encoded = btoa(bytesToBinary(Array.isArray(input) ? input : []));
  return maybeBool(noPad) ? encoded.replace(/=+$/g, "") : encoded;
}

export function Base64Decode(input, noPad) {
  try {
    let normalized = String(input);
    if (maybeBool(noPad)) normalized += "=".repeat((4 - (normalized.length % 4)) % 4);
    return { ok: binaryToBytes(atob(normalized)) };
  } catch (error) {
    return errorResult(error);
  }
}

export function Base64EncodeURL(input, noPad) {
  return toURLBase64(Base64Encode(input, false), noPad);
}

export function Base64DecodeURL(input, noPad) {
  try {
    return { ok: binaryToBytes(atob(fromURLBase64(input, noPad))) };
  } catch (error) {
    return errorResult(error);
  }
}

export function HexEncode(input) {
  return (Array.isArray(input) ? input : []).map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

export function HexDecode(input) {
  input = String(input);
  if (input.length % 2 !== 0 || /[^0-9a-fA-F]/.test(input)) return { err: "invalid hex" };
  const out = [];
  for (let i = 0; i < input.length; i += 2) out.push(Number.parseInt(input.slice(i, i + 2), 16));
  return { ok: out };
}
