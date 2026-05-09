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

function binaryToString(binary) {
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return new TextDecoder().decode(bytes);
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
  const bytes = new TextEncoder().encode(String(input));
  const encoded = btoa(bytesToBinary(bytes));
  return maybeBool(noPad) ? encoded.replace(/=+$/g, "") : encoded;
}

export function Base64Decode(input, noPad) {
  try {
    let normalized = String(input);
    if (maybeBool(noPad)) normalized += "=".repeat((4 - (normalized.length % 4)) % 4);    return { ok: binaryToString(atob(normalized)) };
  } catch (error) {
    return errorResult(error);
  }
}

export function Base64EncodeURL(input, noPad) {
  return toURLBase64(Base64Encode(input, false), noPad);
}

export function Base64DecodeURL(input, noPad) {
  try {
    return { ok: binaryToString(atob(fromURLBase64(input, noPad))) };
  } catch (error) {
    return errorResult(error);
  }
}
