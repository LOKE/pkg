const rfc3339 =
  /^(\d{4})-(\d\d)-(\d\d)[Tt](\d\d):(\d\d):(\d\d)(?:\.(\d+))?([Zz]|[+-]\d\d:\d\d)$/;

export interface Instant {
  seconds: number;
  nanos: number;
}

export function parseTimestamp(text: string): Instant {
  const m = rfc3339.exec(text);
  if (!m) throw new Error(`invalid timestamp ${JSON.stringify(text)}`);

  const date = new Date(0);
  date.setUTCFullYear(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  date.setUTCHours(Number(m[4]), Number(m[5]), Number(m[6]), 0);

  let offsetSeconds = 0;
  if (m[8] !== "Z" && m[8] !== "z") {
    const sign = m[8][0] === "-" ? -1 : 1;
    offsetSeconds = sign * (Number(m[8].slice(1, 3)) * 3600 + Number(m[8].slice(4)) * 60);
  }

  const nanos = m[7] ? Number(m[7].slice(0, 9).padEnd(9, "0")) : 0;
  return { seconds: date.getTime() / 1000 - offsetSeconds, nanos };
}

// Formats like Go's time.Time.MarshalJSON for a UTC time: RFC 3339 with trailing zero nanos trimmed.
export function formatTimestamp(seconds: number, nanos: number): string {
  const base = new Date(seconds * 1000).toISOString().slice(0, 19);
  const fraction = nanos === 0 ? "" : `.${String(nanos).padStart(9, "0").replace(/0+$/, "")}`;
  return `${base}${fraction}Z`;
}

export const zeroTimestamp = "0001-01-01T00:00:00Z";
export const zeroTimeSeconds = -62135596800;
