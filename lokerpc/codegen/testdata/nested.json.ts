import { RPCClient } from "@loke/http-rpc-client";

export type GetUserRequest = {
  id: string;
};

export type GetUserResponse = {
  name: string;
  comments: {
  text: string;
  timestamp: string;
}[];
};

/**
 * 
 */
export class NestedService extends RPCClient {
  constructor(baseUrl: string) {
    super(baseUrl, "nested")
  }
  /**
   * hello1 method
   */
  getUser(req: GetUserRequest): Promise<GetUserResponse> {
    return this.request("getUser", req);
  }
}
