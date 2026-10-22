// Protobuf wire format driven by the JTD schema. Field numbers are the 1-based
// position of each property in byte-wise name order and every message is wrapped
// in an envelope whose field 1 holds the value. The mapping matches the Go
// protocodec byte for byte; see examples/negotiate/README.md.
import { type Codec, type Defs, type Schema, compareUtf8 } from "./schema.ts";
import { formatTimestamp, parseTimestamp, zeroTimeSeconds, zeroTimestamp } from "./timestamp.ts";

const VARINT = 0;
const FIXED64 = 1;
const BYTES = 2;
const FIXED32 = 5;

type Kind =
  | "bool"
  | "int"
  | "uint"
  | "float32"
  | "float64"
  | "string"
  | "timestamp"
  | "struct"
  | "list"
  | "map"
  | "json";

interface Node {
  kind: Kind;
  wire: number;
  min: number;
  max: number;
  fields: FieldPlan[];
  elem: Node;
  elemNullable: boolean;
  // Each element sits in a {1: value} message so null and nested containers stay distinguishable.
  wrap: boolean;
  packed: boolean;
}

interface FieldPlan {
  name: string;
  num: number;
  optional: boolean;
  nullable: boolean;
  node: Node;
}

interface Plan {
  node: Node;
  nullable: boolean;
}

const intRanges: Record<string, [Kind, number, number]> = {
  int8: ["int", -128, 127],
  int16: ["int", -32768, 32767],
  int32: ["int", -2147483648, 2147483647],
  uint8: ["uint", 0, 255],
  uint16: ["uint", 0, 65535],
  uint32: ["uint", 0, 4294967295],
};

function newNode(kind: Kind, wire: number): Node {
  return { kind, wire, min: 0, max: 0, fields: [], elem: undefined as never, elemNullable: false, wrap: false, packed: false };
}

const stringNode = newNode("string", BYTES);

function resolve(schema: Schema, defs: Defs): { schema: Schema; nullable: boolean } {
  let nullable = schema.nullable === true;
  let resolved = schema;
  while (resolved.ref !== undefined) {
    const def = defs[resolved.ref];
    if (!def) throw new Error(`undefined ref ${JSON.stringify(resolved.ref)}`);
    resolved = def;
    nullable ||= def.nullable === true;
  }
  return { schema: resolved, nullable };
}

function compile(schema: Schema, defs: Defs, refs: Map<string, Node>): Plan {
  const { schema: resolved, nullable } = resolve(schema, defs);
  if (schema.ref === undefined) return { node: build(resolved, defs, refs), nullable };

  const cached = refs.get(schema.ref);
  if (cached) return { node: cached, nullable };

  // Registered before building so recursive types refer back to the same node.
  const node = newNode("json", BYTES);
  refs.set(schema.ref, node);
  Object.assign(node, build(resolved, defs, refs, node));
  return { node, nullable };
}

function needsWrap(elem: Schema, defs: Defs): boolean {
  const { schema, nullable } = resolve(elem, defs);
  if (nullable) return true;
  return schema.elements !== undefined || schema.values !== undefined || isJsonForm(schema);
}

const isJsonForm = (s: Schema) =>
  s.type === undefined &&
  s.enum === undefined &&
  s.elements === undefined &&
  s.values === undefined &&
  s.properties === undefined &&
  s.optionalProperties === undefined;

function build(schema: Schema, defs: Defs, refs: Map<string, Node>, into?: Node): Node {
  if (schema.type !== undefined) {
    switch (schema.type) {
      case "boolean":
        return newNode("bool", VARINT);
      case "string":
        return newNode("string", BYTES);
      case "timestamp":
        return newNode("timestamp", BYTES);
      case "float32":
        return newNode("float32", FIXED32);
      case "float64":
        return newNode("float64", FIXED64);
    }
    const range = intRanges[schema.type];
    if (!range) throw new Error(`unsupported schema type ${JSON.stringify(schema.type)}`);
    const node = newNode(range[0], VARINT);
    node.min = range[1];
    node.max = range[2];
    return node;
  }
  if (schema.enum !== undefined) return newNode("string", BYTES);

  if (schema.elements !== undefined) {
    const node = into ?? newNode("list", BYTES);
    node.kind = "list";
    const elem = compile(schema.elements, defs, refs);
    node.elem = elem.node;
    node.elemNullable = elem.nullable;
    node.wrap = needsWrap(schema.elements, defs);
    const resolvedElem = resolve(schema.elements, defs).schema;
    node.packed =
      !node.wrap &&
      resolvedElem.type !== undefined &&
      resolvedElem.type !== "string" &&
      resolvedElem.type !== "timestamp";
    return node;
  }

  if (schema.values !== undefined) {
    const node = into ?? newNode("map", BYTES);
    node.kind = "map";
    const elem = compile(schema.values, defs, refs);
    node.elem = elem.node;
    node.elemNullable = elem.nullable;
    node.wrap = needsWrap(schema.values, defs);
    return node;
  }

  if (schema.properties !== undefined || schema.optionalProperties !== undefined) {
    const node = into ?? newNode("struct", BYTES);
    node.kind = "struct";
    const names = [
      ...Object.keys(schema.properties ?? {}).map((name) => [name, false] as const),
      ...Object.keys(schema.optionalProperties ?? {}).map((name) => [name, true] as const),
    ].sort((a, b) => compareUtf8(a[0], b[0]));

    node.fields = names.map(([name, optional], i) => {
      const prop = (optional ? schema.optionalProperties : schema.properties)![name];
      const compiled = compile(prop, defs, refs);
      return { name, num: i + 1, optional, nullable: compiled.nullable, node: compiled.node };
    });
    return node;
  }

  // The empty and discriminator forms ride along as JSON bytes.
  return newNode("json", BYTES);
}

class Writer {
  buf = Buffer.allocUnsafe(4096);
  pos = 0;

  ensure(n: number) {
    if (this.pos + n <= this.buf.length) return;
    const grown = Buffer.allocUnsafe(Math.max(this.buf.length * 2, this.pos + n));
    this.buf.copy(grown, 0, 0, this.pos);
    this.buf = grown;
  }

  varint(n: number) {
    this.ensure(10);
    const buf = this.buf;
    while (n >= 2147483648) {
      buf[this.pos++] = (n % 128) | 128;
      n = Math.floor(n / 128);
    }
    while (n >= 128) {
      buf[this.pos++] = (n & 127) | 128;
      n >>>= 7;
    }
    buf[this.pos++] = n;
  }

  // Negative int64 values take ten bytes.
  varint64(n: bigint) {
    this.ensure(10);
    let rest = BigInt.asUintN(64, n);
    while (rest >= 128n) {
      this.buf[this.pos++] = Number(rest & 127n) | 128;
      rest >>= 7n;
    }
    this.buf[this.pos++] = Number(rest);
  }

  tag(num: number, wire: number) {
    this.varint(num * 8 + wire);
  }

  string(s: string) {
    const length = Buffer.byteLength(s);
    this.varint(length);
    this.ensure(length);
    this.buf.write(s, this.pos, length, "utf8");
    this.pos += length;
  }

  float32(n: number) {
    this.ensure(4);
    this.buf.writeFloatLE(n, this.pos);
    this.pos += 4;
  }

  float64(n: number) {
    this.ensure(8);
    this.buf.writeDoubleLE(n, this.pos);
    this.pos += 8;
  }

  // Reserves a one byte length prefix that end() widens if the body turns out longer.
  begin(): number {
    this.ensure(1);
    return ++this.pos;
  }

  end(mark: number) {
    const length = this.pos - mark;
    if (length < 128) {
      this.buf[mark - 1] = length;
      return;
    }
    const size = length < 16384 ? 2 : length < 2097152 ? 3 : length < 268435456 ? 4 : 5;
    this.ensure(size);
    this.buf.copyWithin(mark - 1 + size, mark, this.pos);
    this.pos += size - 1;
    const bodyEnd = this.pos;
    this.pos = mark - 1;
    this.varint(length);
    this.pos = bodyEnd;
  }
}

function isZero(node: Node, value: unknown): boolean {
  switch (node.kind) {
    case "bool":
      return value === false;
    case "string":
      return value === "";
    case "timestamp": {
      const { seconds, nanos } = parseTimestamp(String(value));
      return seconds === zeroTimeSeconds && nanos === 0;
    }
    case "float32":
    case "float64":
      return Object.is(value, 0);
    default:
      return value === 0;
  }
}

// Proto3 presence: null and undefined are omitted, and a non-null value of an
// optional or nullable property is always written, like a non-nil Go pointer.
function writeField(w: Writer, num: number, node: Node, explicit: boolean, value: unknown) {
  if (value === null || value === undefined) return;

  switch (node.kind) {
    case "list":
      return writeList(w, num, node, value);
    case "map":
      return writeMap(w, num, node, value);
    case "struct":
    case "json":
      break;
    default:
      if (!explicit && isZero(node, value)) return;
  }
  w.tag(num, node.wire);
  writePayload(w, node, value);
}

function writePayload(w: Writer, node: Node, value: unknown) {
  switch (node.kind) {
    case "bool":
      if (typeof value !== "boolean") throw new Error(`expected boolean, got ${typeof value}`);
      w.varint(value ? 1 : 0);
      return;
    case "int":
    case "uint": {
      const n = value as number;
      if (!Number.isInteger(n)) throw new Error(`expected integer, got ${JSON.stringify(value)}`);
      if (n < node.min || n > node.max) throw new Error(`value ${n} out of range`);
      // zigzag
      w.varint(node.kind === "int" ? (n < 0 ? -2 * n - 1 : 2 * n) : n);
      return;
    }
    case "float32":
    case "float64":
      if (typeof value !== "number") throw new Error(`expected number, got ${typeof value}`);
      if (node.kind === "float32") w.float32(value);
      else w.float64(value);
      return;
    case "string":
      if (typeof value !== "string") throw new Error(`expected string, got ${typeof value}`);
      w.string(value);
      return;
    case "timestamp": {
      const { seconds, nanos } = parseTimestamp(String(value));
      const mark = w.begin();
      if (seconds !== 0) {
        w.tag(1, VARINT);
        w.varint64(BigInt(seconds));
      }
      if (nanos !== 0) {
        w.tag(2, VARINT);
        w.varint(nanos);
      }
      w.end(mark);
      return;
    }
    case "struct": {
      if (typeof value !== "object" || Array.isArray(value)) throw new Error(`expected object, got ${JSON.stringify(value)}`);
      const obj = value as Record<string, unknown>;
      const mark = w.begin();
      for (const f of node.fields) {
        try {
          writeField(w, f.num, f.node, f.optional || f.nullable, obj[f.name]);
        } catch (err) {
          throw new Error(`${f.name}: ${(err as Error).message}`);
        }
      }
      w.end(mark);
      return;
    }
    case "json":
      w.string(JSON.stringify(value));
      return;
  }
  throw new Error(`kind ${node.kind} has no scalar payload`);
}

function writeList(w: Writer, num: number, node: Node, value: unknown) {
  if (!Array.isArray(value)) throw new Error(`expected array, got ${typeof value}`);
  if (value.length === 0) return;

  const elem = node.elem;
  if (node.packed) {
    w.tag(num, BYTES);
    const mark = w.begin();
    for (let i = 0; i < value.length; i++) {
      if (value[i] === null || value[i] === undefined) throw new Error(`[${i}]: null element in non-nullable list`);
      writePayload(w, elem, value[i]);
    }
    w.end(mark);
    return;
  }

  for (let i = 0; i < value.length; i++) {
    try {
      if (node.wrap) {
        w.tag(num, BYTES);
        const mark = w.begin();
        writeField(w, 1, elem, node.elemNullable, value[i]);
        w.end(mark);
      } else {
        if (value[i] === null || value[i] === undefined) throw new Error("null element in non-nullable list");
        w.tag(num, elem.wire);
        writePayload(w, elem, value[i]);
      }
    } catch (err) {
      throw new Error(`[${i}]: ${(err as Error).message}`);
    }
  }
}

function writeMap(w: Writer, num: number, node: Node, value: unknown) {
  if (typeof value !== "object" || Array.isArray(value)) throw new Error(`expected object, got ${typeof value}`);
  const obj = value as Record<string, unknown>;
  for (const key of Object.keys(obj).sort(compareUtf8)) {
    try {
      w.tag(num, BYTES);
      const mark = w.begin();
      writeField(w, 1, stringNode, false, key);
      if (node.wrap) {
        w.tag(2, BYTES);
        const wrapperMark = w.begin();
        writeField(w, 1, node.elem, node.elemNullable, obj[key]);
        w.end(wrapperMark);
      } else {
        writeField(w, 2, node.elem, node.elemNullable, obj[key]);
      }
      w.end(mark);
    } catch (err) {
      throw new Error(`[${key}]: ${(err as Error).message}`);
    }
  }
}

class Reader {
  pos = 0;
  constructor(
    readonly buf: Buffer,
    readonly end: number,
  ) {}

  varint(): number {
    let result = 0;
    let scale = 1;
    for (let i = 0; i < 10; i++) {
      if (this.pos >= this.end) throw new Error("truncated message");
      const b = this.buf[this.pos++];
      result += (b & 127) * scale;
      if (b < 128) return result;
      scale *= 128;
    }
    throw new Error("varint too long");
  }

  // Two's complement int64, for timestamp seconds.
  varint64(): bigint {
    let result = 0n;
    for (let i = 0; i < 10; i++) {
      if (this.pos >= this.end) throw new Error("truncated message");
      const b = this.buf[this.pos++];
      result |= BigInt(b & 127) << BigInt(7 * i);
      if (b < 128) return BigInt.asIntN(64, result);
    }
    throw new Error("varint too long");
  }

  take(n: number): number {
    if (n > this.end - this.pos) throw new Error("truncated message");
    const start = this.pos;
    this.pos += n;
    return start;
  }

  // Reads a length prefix and returns where that message ends.
  length(): number {
    const n = this.varint();
    if (n > this.end - this.pos) throw new Error("truncated message");
    return this.pos + n;
  }

  string(): string {
    const end = this.length();
    const text = this.buf.toString("utf8", this.pos, end);
    this.pos = end;
    return text;
  }

  skip(wire: number) {
    switch (wire) {
      case VARINT:
        this.varint();
        return;
      case FIXED64:
        this.take(8);
        return;
      case BYTES:
        this.pos = this.length();
        return;
      case FIXED32:
        this.take(4);
        return;
    }
    throw new Error(`unsupported wire type ${wire}`);
  }

  tag(): { num: number; wire: number } {
    const tag = this.varint();
    return { num: Math.floor(tag / 8), wire: tag % 8 };
  }
}

// Absent lists and maps decode as empty even when nullable: the wire cannot tell nil from empty.
function defaultValue(node: Node, nullable: boolean, depth = 0): unknown {
  if (node.kind === "list") return [];
  if (node.kind === "map") return {};
  if (nullable) return null;
  switch (node.kind) {
    case "bool":
      return false;
    case "string":
      return "";
    case "timestamp":
      return zeroTimestamp;
    case "json":
      return null;
    case "struct": {
      if (depth > 16) return null;
      const obj: Record<string, unknown> = {};
      for (const f of node.fields) {
        if (!f.optional) obj[f.name] = defaultValue(f.node, f.nullable, depth + 1);
      }
      return obj;
    }
  }
  return 0;
}

function expect(wire: number, want: number) {
  if (wire !== want) throw new Error(`wire type ${wire}, want ${want}`);
}

function readScalar(r: Reader, node: Node): unknown {
  switch (node.kind) {
    case "bool":
      return r.varint() !== 0;
    case "int": {
      const raw = r.varint();
      const n = raw % 2 === 0 ? raw / 2 : -(raw + 1) / 2;
      if (n < node.min || n > node.max) throw new Error(`value ${n} out of range`);
      return n;
    }
    case "uint": {
      const n = r.varint();
      if (n > node.max) throw new Error(`value ${n} out of range`);
      return n;
    }
    case "float32": {
      const start = r.take(4);
      return r.buf.readFloatLE(start);
    }
    case "float64": {
      const start = r.take(8);
      return r.buf.readDoubleLE(start);
    }
  }
  throw new Error(`kind ${node.kind} is not a scalar`);
}

function readTimestamp(r: Reader): string {
  const end = r.length();
  let seconds = 0;
  let nanos = 0;
  while (r.pos < end) {
    const { num, wire } = r.tag();
    if ((num !== 1 && num !== 2) || wire !== VARINT) {
      if (num === 1 || num === 2) throw new Error(`timestamp wire type ${wire}, want varint`);
      r.skip(wire);
    } else if (num === 1) {
      seconds = Number(r.varint64());
    } else {
      nanos = r.varint();
    }
  }
  if (r.pos !== end) throw new Error("truncated message");
  return formatTimestamp(seconds, nanos);
}

function readSingle(r: Reader, node: Node, wire: number): unknown {
  expect(wire, node.wire);
  switch (node.kind) {
    case "string":
      return r.string();
    case "timestamp":
      return readTimestamp(r);
    case "json":
      return JSON.parse(r.string());
    case "struct":
      return readStruct(r, node);
  }
  return readScalar(r, node);
}

function readStruct(r: Reader, node: Node): Record<string, unknown> {
  const end = r.length();
  const obj: Record<string, unknown> = {};
  const fields = node.fields;

  while (r.pos < end) {
    const tag = r.varint();
    const wire = tag % 8;
    const f = fields[Math.floor(tag / 8) - 1];
    if (f === undefined) {
      r.skip(wire);
      continue;
    }
    try {
      obj[f.name] = readValue(r, f.node, wire, obj[f.name]);
    } catch (err) {
      throw new Error(`${f.name}: ${(err as Error).message}`);
    }
  }
  if (r.pos !== end) throw new Error("truncated message");

  for (const f of fields) {
    if (!f.optional && !Object.hasOwn(obj, f.name)) obj[f.name] = defaultValue(f.node, f.nullable);
  }
  return obj;
}

// Lists and maps accumulate over repeated occurrences of their field.
function readValue(r: Reader, node: Node, wire: number, current: unknown): unknown {
  switch (node.kind) {
    case "list": {
      const list = (current as unknown[] | undefined) ?? [];
      readListItem(r, node, wire, list);
      return list;
    }
    case "map": {
      const map = (current as Record<string, unknown> | undefined) ?? {};
      readMapEntry(r, node, wire, map);
      return map;
    }
  }
  return readSingle(r, node, wire);
}

function readWrapper(r: Reader, node: Node, nullable: boolean): unknown {
  const end = r.length();
  let value: unknown;
  while (r.pos < end) {
    const { num, wire } = r.tag();
    if (num === 1) value = readValue(r, node, wire, value);
    else r.skip(wire);
  }
  if (r.pos !== end) throw new Error("truncated message");
  return value === undefined ? defaultValue(node, nullable) : value;
}

function readListItem(r: Reader, node: Node, wire: number, list: unknown[]) {
  const elem = node.elem;
  if (node.packed && wire === BYTES) {
    const end = r.length();
    while (r.pos < end) list.push(readScalar(r, elem));
    if (r.pos !== end) throw new Error("truncated message");
    return;
  }
  if (node.wrap) {
    expect(wire, BYTES);
    list.push(readWrapper(r, elem, node.elemNullable));
    return;
  }
  list.push(readSingle(r, elem, wire));
}

function readMapEntry(r: Reader, node: Node, wire: number, map: Record<string, unknown>) {
  expect(wire, BYTES);
  const end = r.length();
  let key = "";
  let value: unknown;
  let hasValue = false;

  while (r.pos < end) {
    const field = r.tag();
    if (field.num === 1) {
      key = readSingle(r, stringNode, field.wire) as string;
    } else if (field.num === 2) {
      hasValue = true;
      if (node.wrap) {
        expect(field.wire, BYTES);
        value = readWrapper(r, node.elem, node.elemNullable);
      } else {
        value = readValue(r, node.elem, field.wire, undefined);
      }
    } else {
      r.skip(field.wire);
    }
  }
  if (r.pos !== end) throw new Error("truncated message");

  Object.defineProperty(map, key, {
    value: hasValue ? value : defaultValue(node.elem, node.elemNullable),
    enumerable: true,
    writable: true,
    configurable: true,
  });
}

export class ProtobufCodec implements Codec {
  readonly contentType = "application/x-protobuf";
  readonly schemaFree = false;
  #plans = new WeakMap<Schema, Plan>();

  #plan(schema: Schema, defs: Defs): Plan {
    let plan = this.#plans.get(schema);
    if (!plan) {
      plan = compile(schema, defs, new Map());
      this.#plans.set(schema, plan);
    }
    return plan;
  }

  encode(value: unknown, schema: Schema, defs: Defs): Uint8Array {
    const plan = this.#plan(schema, defs);
    const w = new Writer();
    writeField(w, 1, plan.node, plan.nullable, value);
    return w.buf.subarray(0, w.pos);
  }

  decode(data: Uint8Array, schema: Schema, defs: Defs): unknown {
    const plan = this.#plan(schema, defs);
    const r = new Reader(Buffer.from(data.buffer, data.byteOffset, data.byteLength), data.byteLength);
    let value: unknown;
    while (r.pos < r.end) {
      const { num, wire } = r.tag();
      if (num === 1) value = readValue(r, plan.node, wire, value);
      else r.skip(wire);
    }
    return value === undefined ? defaultValue(plan.node, plan.nullable) : value;
  }
}
