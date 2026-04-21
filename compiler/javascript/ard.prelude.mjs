export function makeArdError(kind, moduleName, fnName, line, message, extra = {}) {
  const error = new globalThis.Error(message);
  error.ard_error = kind;
  error.module = moduleName;
  error.function = fnName;
  error.line = line;
  for (const key in extra) error[key] = extra[key];
  return error;
}

export function makeTryReturn(value) {
  return { __ard_try_return: true, value };
}

export function makeBreakSignal() {
  return { __ard_break: true };
}

const ARD_ENUM = Symbol.for("ard.enum");

export function makeEnum(enumName, variantName, value) {
  return Object.freeze({ [ARD_ENUM]: true, enum: enumName, variant: variantName, value });
}

export function isArdEnum(value) {
  return !!(value && typeof value === "object" && value[ARD_ENUM] === true);
}

export function isEnumOf(value, enumName) {
  return isArdEnum(value) && value.enum === enumName;
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
