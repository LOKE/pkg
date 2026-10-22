// Binary codec negotiation on top of @loke/http-rpc-client. JSON calls go
// through the stock RPCClient untouched. The stock client hard-codes JSON in its
// fetch call, so binary calls make their own request and reuse its error types.
import { RPCClient, RpcHTTPError, RpcResponseError } from "@loke/http-rpc-client";
import type { Codec, Defs, Schema } from "./schema.ts";

const defaultProbeMaxAgeMs = 5 * 60 * 1000;
const failedProbeMaxAgeMs = 30 * 1000;
const defaultTimeoutMs = 60 * 1000;

interface Endpoint {
  request?: Schema;
  response?: Schema;
}

// A missing codec means plain JSON.
interface Negotiated {
  codec?: Codec;
  etag?: string;
  endpoints: Map<string, Endpoint>;
  defs: Defs;
  expiresAt: number;
}

export interface NegotiatingClientOptions {
  // Codecs in order of preference; the first one the server lists is used.
  codecs: Codec[];
}

function maxAgeMs(cacheControl: string | null): number {
  for (const directive of (cacheControl ?? "").split(",")) {
    const match = /^max-age=(\d+)$/.exec(directive.trim());
    if (match) return Number(match[1]) * 1000;
  }
  return defaultProbeMaxAgeMs;
}

function mediaType(header: string | null | undefined): string {
  return (header ?? "").split(";")[0].trim().toLowerCase();
}

// A 412, or a 400/415 that is not JSON, is how a replica without codec support answers a binary body.
function binaryRejected(res: Response): boolean {
  if (res.status === 412) return true;
  return (res.status === 400 || res.status === 415) && mediaType(res.headers.get("content-type")) !== "application/json";
}

export class NegotiatingClient extends RPCClient {
  readonly #base: string;
  readonly #serviceName: string;
  readonly #codecs: Codec[];
  #current?: Negotiated;
  #probing?: Promise<Negotiated>;

  constructor(baseURL: string, serviceName: string, options: NegotiatingClientOptions) {
    super(baseURL, serviceName);
    this.#base = baseURL.endsWith("/") ? baseURL : `${baseURL}/`;
    this.#serviceName = serviceName;
    this.#codecs = options.codecs;
  }

  async #negotiate(): Promise<Negotiated> {
    if (this.#current && Date.now() < this.#current.expiresAt) return this.#current;
    this.#probing ??= this.#probe().finally(() => {
      this.#probing = undefined;
    });
    this.#current = await this.#probing;
    return this.#current;
  }

  async #probe(): Promise<Negotiated> {
    const json: Negotiated = { endpoints: new Map(), defs: {}, expiresAt: 0 };

    let res: Response;
    try {
      res = await fetch(this.#base.slice(0, -1), { method: "OPTIONS", signal: AbortSignal.timeout(defaultTimeoutMs) });
    } catch {
      return { ...json, expiresAt: Date.now() + failedProbeMaxAgeMs };
    }
    json.expiresAt = Date.now() + maxAgeMs(res.headers.get("cache-control"));
    if (!res.ok) {
      await res.body?.cancel();
      return json;
    }

    const serverTypes = (res.headers.get("accept-post") ?? "").split(",").map(mediaType);
    const codec = this.#codecs.find((candidate) => serverTypes.includes(candidate.contentType));
    const etag = res.headers.get("etag");
    if (!codec || !etag) {
      await res.body?.cancel();
      return json;
    }

    try {
      const meta = (await res.json()) as {
        interfaces: { methodName: string; requestTypeDef?: Schema; responseTypeDef?: Schema }[];
        definitions?: Defs;
      };
      const endpoints = new Map(
        meta.interfaces.map((i) => [i.methodName, { request: i.requestTypeDef, response: i.responseTypeDef }]),
      );
      return { ...json, codec, etag, endpoints, defs: meta.definitions ?? {} };
    } catch {
      return json;
    }
  }

  async request(methodName: string, params: Record<string, unknown>, options: { timeout?: number } = {}): Promise<any> {
    const session = await this.#negotiate();
    const endpoint = session.endpoints.get(methodName);
    if (!session.codec || !endpoint?.request) return super.request(methodName, params, options);

    const res = await this.#send(session.codec, session, endpoint, methodName, params, options.timeout ?? defaultTimeoutMs);
    if (binaryRejected(res)) {
      await res.body?.cancel();
      if (this.#current === session) this.#current = undefined;
      return super.request(methodName, params, options);
    }
    return this.#handle(session.codec, session, endpoint, methodName, res);
  }

  async #send(
    codec: Codec,
    session: Negotiated,
    endpoint: Endpoint,
    methodName: string,
    params: Record<string, unknown>,
    timeoutMs: number,
  ): Promise<Response> {
    const headers: Record<string, string> = {
      "Content-Type": codec.contentType,
      "X-Request-Deadline": new Date(Date.now() + timeoutMs).toISOString(),
    };
    if (!codec.schemaFree) headers["If-Match"] = session.etag!;
    if (endpoint.response) headers.Accept = `${codec.contentType}, application/json;q=0.5`;

    const body = codec.encode(params, endpoint.request!, session.defs);
    try {
      return await fetch(new URL(methodName, this.#base), {
        method: "POST",
        body: body as BodyInit,
        headers,
        signal: AbortSignal.timeout(timeoutMs),
      });
    } catch (err) {
      if ((err as Error).name === "AbortError" || (err as Error).name === "TimeoutError") throw err;
      throw new RpcHTTPError(`request failed [${this.#base}].${methodName}`, { cause: err });
    }
  }

  async #handle(codec: Codec, session: Negotiated, endpoint: Endpoint, methodName: string, res: Response): Promise<any> {
    if (res.ok) {
      if (res.status === 204 || res.headers.get("content-length") === "0") return undefined;
      if (mediaType(res.headers.get("content-type")) !== codec.contentType) return res.json();
      if (!endpoint.response) throw new Error(`unexpected ${codec.contentType} response`);
      const data = Buffer.from(await res.arrayBuffer());
      if (data.length === 0) return undefined;
      try {
        return codec.decode(data, endpoint.response, session.defs);
      } catch (err) {
        throw new Error(`Error decoding rpc response: ${(err as Error).message}`, { cause: err });
      }
    }

    let errResult: Record<string, unknown>;
    if (res.headers.get("content-type")?.includes("application/json")) {
      errResult = (await res.json()) as Record<string, unknown>;
    } else {
      errResult = { expose: false, message: await res.text() };
    }

    const source = `${this.#serviceName}/${methodName}`;
    if (!errResult.type) {
      throw Object.assign(new Error(errResult.message as string), {
        code: errResult.code,
        expose: errResult.expose,
        source: [source],
      });
    }
    throw new RpcResponseError(source, errResult);
  }
}
