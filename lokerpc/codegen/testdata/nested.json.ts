import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type GetUserRequest = {
  id: string;
};

export type GetUserResponse = {
  comments: ({
  text: string;
  timestamp: string;
} | null)[];
  name: string;
};

/**
 * 
 */
export class NestedService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "nested")
  }
  /**
   * hello1 method
   */
  getUser(ctx: Context, req: GetUserRequest): Promise<GetUserResponse> {
    return this.request(ctx, "getUser", req);
  }
}
