import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type GetUserRequest = {
  name: string;
};

export type GetUserResponse = {
  name: string;
};

export type GetUserRequest_ = {
  id: string;
};

export type GetUserResponse_ = {
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
  getUser(ctx: Context, req: GetUserRequest_): Promise<GetUserResponse_> {
    return this.request(ctx, "getUser", req);
  }
}
