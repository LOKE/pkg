import { RPCClient } from "@loke/http-rpc-client";

export class Service1Service extends RPCClient {
  constructor(baseUrl: string) {
    super(baseUrl, "service1")
  }
  hello1(req: any): Promise<any> {
    return this.request("hello1", req);
  }
}
