import assert from "node:assert/strict";
import { before, describe, it } from "node:test";
import { MsgpackCodec } from "../src/msgpack.ts";
import { ProtobufCodec } from "../src/protobuf.ts";
import type { Defs, Schema } from "../src/schema.ts";
import { formatTimestamp, parseTimestamp } from "../src/timestamp.ts";
import { type Dump, goDump, normalize } from "./helpers.ts";

const proto = new ProtobufCodec();
const msgpack = new MsgpackCodec();

describe("against the Go codecs", () => {
  let dump: Dump;
  before(() => {
    dump = goDump();
  });

  it("writes the same protobuf bytes as Go", () => {
    for (const c of dump.cases) {
      const { schema, defs } = dump.schemas[c.schema];
      assert.equal(Buffer.from(proto.encode(c.json, schema, defs)).toString("hex"), c.proto, c.name);
    }
  });

  it("reads Go's protobuf bytes", () => {
    for (const c of dump.cases) {
      const { schema, defs } = dump.schemas[c.schema];
      assert.deepEqual(normalize(proto.decode(Buffer.from(c.proto, "hex"), schema, defs)), normalize(c.json), c.name);
    }
  });

  it("reads Go's msgpack bytes", () => {
    for (const c of dump.cases) {
      const got = msgpack.decode(Buffer.from(c.msgpack, "hex")) as Record<string, unknown>;
      // Go carries netip.Addr as raw bytes while the schema says string.
      const want = { ...(c.json as Record<string, unknown>) };
      if ("addr" in want) {
        assert.ok(got.addr instanceof Uint8Array, `${c.name}: addr`);
        delete want.addr;
        delete got.addr;
      }
      assert.deepEqual(normalize(got), normalize(want), c.name);
    }
  });

  it("writes msgpack that decodes back to the same value", () => {
    for (const c of dump.cases) {
      const { schema, defs } = dump.schemas[c.schema];
      const back = msgpack.decode(msgpack.encode(c.json, schema, defs));
      assert.deepEqual(back, c.json, c.name);
    }
  });
});

describe("timestamps", () => {
  const cases: [string, number, number][] = [
    ["1970-01-01T00:00:00Z", 0, 0],
    ["2026-07-08T09:10:11.123456789Z", 1783501811, 123456789],
    ["2026-07-08T19:10:11.5+10:00", 1783501811, 500000000],
    ["0001-01-01T00:00:00Z", -62135596800, 0],
    ["1901-02-03T04:05:06.000000007-00:00", -2174586894, 7],
  ];

  for (const [text, seconds, nanos] of cases) {
    it(`parses ${text}`, () => {
      assert.deepEqual(parseTimestamp(text), { seconds, nanos });
    });
  }

  it("formats UTC with trailing zeros trimmed", () => {
    assert.equal(formatTimestamp(1783501811, 500000000), "2026-07-08T09:10:11.5Z");
    assert.equal(formatTimestamp(1783501811, 0), "2026-07-08T09:10:11Z");
    assert.equal(formatTimestamp(-62135596800, 0), "0001-01-01T00:00:00Z");
  });

  for (const bad of ["", "2026-07-08", "2026-07-08T09:10:11", "yesterday"]) {
    it(`rejects ${JSON.stringify(bad)}`, () => {
      assert.throws(() => parseTimestamp(bad), /invalid timestamp/);
    });
  }
});

describe("protobuf decoding of bad input", () => {
  const schema: Schema = { ref: "Thing" };
  const defs: Defs = {
    Thing: { properties: { id: { type: "int32" }, name: { type: "string" }, tags: { elements: { type: "string" } } }, optionalProperties: { note: { type: "string" } } },
  };
  const message = (...bytes: number[]) => Buffer.from(bytes);

  const bad: [string, Buffer, RegExp][] = [
    ["truncated length", message(0x0a, 0x05, 0x08), /truncated/],
    ["truncated varint", message(0x0a, 0x01, 0x80), /truncated/],
    ["varint longer than ten bytes", message(0x0a, 0x0c, ...Array(11).fill(0x80), 0x00), /varint too long/],
    ["string where a varint belongs", message(0x0a, 0x04, 0x0a, 0x02, 0x68, 0x69), /id: wire type 2, want 0/],
    ["varint where a string belongs", message(0x0a, 0x02, 0x10, 0x01), /name: wire type 0, want 2/],
    ["int outside int32", message(0x0a, 0x06, 0x08, 0xff, 0xff, 0xff, 0xff, 0x1f), /out of range/],
    ["group wire type", message(0x13), /unsupported wire type 3/],
    ["length past the end", message(0x0a, 0x7f, 0x01), /truncated/],
  ];

  for (const [name, bytes, pattern] of bad) {
    it(`rejects ${name}`, () => {
      assert.throws(() => proto.decode(bytes, schema, defs), pattern);
    });
  }

  it("skips unknown fields of every wire type", () => {
    const body = message(0x08, 0x04, 0x28, 0x01, 0x31, 0, 0, 0, 0, 0, 0, 0, 0, 0x3d, 0, 0, 0, 0, 0x42, 0x01, 0x7a, 0x12, 0x02, 0x68, 0x69);
    const bytes = Buffer.concat([message(0x0a, body.length), body, message(0x7a, 0x01, 0x00)]);
    assert.deepEqual(proto.decode(bytes, schema, defs), { id: 2, name: "hi", tags: [] });
  });

  it("fills absent required properties with zero values and leaves optional ones out", () => {
    assert.deepEqual(proto.decode(Buffer.alloc(0), schema, defs), { id: 0, name: "", tags: [] });
  });

  it("refuses to encode values outside the schema", () => {
    assert.throws(() => proto.encode({ id: 2 ** 31, name: "", tags: [] }, schema, defs), /id: value 2147483648 out of range/);
    assert.throws(() => proto.encode({ id: 1.5, name: "", tags: [] }, schema, defs), /id: expected integer/);
    assert.throws(() => proto.encode({ id: 1, name: 7, tags: [] }, schema, defs), /name: expected string/);
    assert.throws(() => proto.encode({ id: 1, name: "", tags: [null] }, schema, defs), /tags: \[0\]: null element/);
    assert.throws(() => proto.encode([], schema, defs), /expected object/);
  });
});
