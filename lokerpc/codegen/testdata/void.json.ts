import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type Hello1Response = any;

/**
 * hello
 */
export class Service1Service extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "service1")
  }
  /**
   * hello1 method
   */
  hello1(ctx: Context, req: any): Promise<void> {
    return this.request(ctx, "hello1", req);
  }
}
