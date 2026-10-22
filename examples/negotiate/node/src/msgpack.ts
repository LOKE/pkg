// MessagePack keyed by JSON field names, so it needs no schema to decode. The
// schema only picks the wire form of values JSON leaves open: timestamps become
// the MessagePack timestamp extension, which is what the Go codec writes for
// time.Time, and float32 properties are written as float32, which Go requires.
import {
  Decoder,
  EXT_TIMESTAMP,
  Encoder,
  ExtensionCodec,
  decodeTimestampToTimeSpec,
  encodeTimeSpecToTimestamp,
} from "@msgpack/msgpack";
import type { Codec, Defs, Schema } from "./schema.ts";
import { formatTimestamp, parseTimestamp } from "./timestamp.ts";

class Instant {
  constructor(
    readonly sec: number,
    readonly nsec: number,
  ) {}
}

class Float32 {
  constructor(readonly value: number) {}
}

const extensionCodec = new ExtensionCodec();
extensionCodec.register({
  type: EXT_TIMESTAMP,
  encode: (value) => (value instanceof Instant ? encodeTimeSpecToTimestamp(value) : null),
  decode: (data) => {
    const { sec, nsec } = decodeTimestampToTimeSpec(data);
    return formatTimestamp(sec, nsec);
  },
});

function resolve(schema: Schema, defs: Defs): Schema {
  let resolved = schema;
  while (resolved.ref !== undefined) {
    const def = defs[resolved.ref];
    if (!def) throw new Error(`undefined ref ${JSON.stringify(resolved.ref)}`);
    resolved = def;
  }
  return resolved;
}

function children(schema: Schema): Schema[] {
  return [
    ...(schema.elements ? [schema.elements] : []),
    ...(schema.values ? [schema.values] : []),
    ...Object.values(schema.properties ?? {}),
    ...Object.values(schema.optionalProperties ?? {}),
    ...Object.values(schema.mapping ?? {}).flatMap((variant) => children(variant)),
  ];
}

function needsConversion(schema: Schema, defs: Defs, visiting: Set<string>): boolean {
  if (schema.ref !== undefined) {
    if (visiting.has(schema.ref)) return false;
    visiting.add(schema.ref);
  }
  const resolved = resolve(schema, defs);
  return (
    resolved.type === "timestamp" ||
    resolved.type === "float32" ||
    children(resolved).some((child) => needsConversion(child, defs, visiting))
  );
}

function toWire(value: unknown, schema: Schema, defs: Defs): unknown {
  if (value === null || value === undefined) return value;
  const s = resolve(schema, defs);

  if (s.type === "timestamp") {
    if (typeof value !== "string") return value;
    const { seconds, nanos } = parseTimestamp(value);
    return new Instant(seconds, nanos);
  }
  if (s.type === "float32") return typeof value === "number" ? new Float32(value) : value;
  if (s.elements && Array.isArray(value)) return value.map((item) => toWire(item, s.elements!, defs));
  if (typeof value !== "object" || Array.isArray(value)) return value;

  const obj = value as Record<string, unknown>;
  if (s.values) {
    return Object.fromEntries(Object.entries(obj).map(([key, item]) => [key, toWire(item, s.values!, defs)]));
  }
  const properties =
    s.discriminator !== undefined
      ? (s.mapping?.[String(obj[s.discriminator])] ?? {})
      : s;
  const out: Record<string, unknown> = { ...obj };
  for (const group of [properties.properties, properties.optionalProperties]) {
    for (const [name, prop] of Object.entries(group ?? {})) {
      if (name in obj) out[name] = toWire(obj[name], prop, defs);
    }
  }
  return out;
}

// The library has no way to write one number as float32 and the rest as float64, so Float32
// is intercepted in doEncode, which the typings hide. Pinned to @msgpack/msgpack 3.1.3.
function newEncoder(): Encoder {
  const encoder = new Encoder({ extensionCodec, ignoreUndefined: true });
  const internals = encoder as any;
  const doEncode = internals.doEncode;
  internals.doEncode = function (this: any, object: unknown, depth: number) {
    if (!(object instanceof Float32)) return doEncode.call(this, object, depth);
    this.ensureBufferSizeToWrite(5);
    this.writeU8(0xca);
    this.writeF32(object.value);
  };
  return encoder;
}

export class MsgpackCodec implements Codec {
  readonly contentType = "application/vnd.msgpack";
  readonly schemaFree = true;
  #encoder = newEncoder();
  #decoder = new Decoder({ extensionCodec });
  #needsConversion = new WeakMap<Schema, boolean>();

  encode(value: unknown, schema: Schema, defs: Defs): Uint8Array {
    let convert = this.#needsConversion.get(schema);
    if (convert === undefined) {
      convert = needsConversion(schema, defs, new Set());
      this.#needsConversion.set(schema, convert);
    }
    return this.#encoder.encode(convert ? toWire(value, schema, defs) : value);
  }

  decode(data: Uint8Array): unknown {
    return this.#decoder.decode(data);
  }
}
