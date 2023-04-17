import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type User = {
  "Email@Address": string;
  "First-Name": string;
  "Last Name": string;
  "User_ID": string;
};

export type GetUserRequest = {
  id: string;
};

/**
 * 
 */
export class HyphenatedServiceNameService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "hyphenated-service-name")
  }
  /**
   * hello1 method
   */
  getUser(ctx: Context, req: GetUserRequest): Promise<User> {
    return this.request(ctx, "getUser", req);
  }
}
