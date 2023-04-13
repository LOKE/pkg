import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type User = {
  anything: any;
  name: string;
  anythingElse?: any;
};

export type GetUserRequest = {
  id: string;
};

/**
 * 
 */
export class TypedService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "typed")
  }
  /**
   * hello1 method
   */
  getUser(ctx: Context, req: GetUserRequest): Promise<User> {
    return this.request(ctx, "getUser", req);
  }
}
