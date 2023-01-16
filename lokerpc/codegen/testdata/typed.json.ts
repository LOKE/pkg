import { RPCClient } from "@loke/http-rpc-client";

export type User = {
  name: string;
  anything: any;
  anythingElse?: any;
};

export type GetUserRequest = {
  id: string;
};

export type GetUserResponse = User;

/**
 * 
 */
export class TypedService extends RPCClient {
  constructor(baseUrl: string) {
    super(baseUrl, "typed")
  }
  /**
   * hello1 method
   */
  getUser(req: GetUserRequest): Promise<GetUserResponse> {
    return this.request("getUser", req);
  }
}
