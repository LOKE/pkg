export interface Schema {
  ref?: string;
  type?: string;
  enum?: string[];
  elements?: Schema;
  values?: Schema;
  properties?: Record<string, Schema>;
  optionalProperties?: Record<string, Schema>;
  discriminator?: string;
  mapping?: Record<string, Schema>;
  nullable?: boolean;
  metadata?: Record<string, unknown>;
}

export type Defs = Record<string, Schema>;

// A codec turns JSON-shaped values into wire bytes and back, driven by the JTD schema.
export interface Codec {
  readonly contentType: string;
  // Schema free formats carry field names, so they need no If-Match and never get a 412.
  readonly schemaFree: boolean;
  encode(value: unknown, schema: Schema, defs: Defs): Uint8Array;
  decode(data: Uint8Array, schema: Schema, defs: Defs): unknown;
}

export const isVoid = (schema: Schema) => schema.metadata?.void === true;

// Byte-wise UTF-8 order, which is how the Go codec sorts property names and map keys.
export function compareUtf8(a: string, b: string): number {
  return Buffer.compare(Buffer.from(a), Buffer.from(b));
}
