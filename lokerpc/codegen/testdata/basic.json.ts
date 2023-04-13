import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

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
  hello1(ctx: Context, req: any): Promise<any> {
    return this.request(ctx, "hello1", req);
  }
}
