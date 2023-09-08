import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type Hello1Request = {
  thing: 
| {
  eventType: "USER_CREATED";
  id: string;
}
| {
  eventType: "USER_DELETED";
  id: string;
  softDelete: boolean;
}
| {
  eventType: "USER_PAYMENT_PLAN_CHANGED";
  id: string;
  plan: "FREE" | "PAID";
};
};

export type Hello1Response = 
| {
  eventType: "USER_CREATED";
  id: string;
}
| {
  eventType: "USER_DELETED";
  id: string;
  softDelete: boolean;
}
| {
  eventType: "USER_PAYMENT_PLAN_CHANGED";
  id: string;
  plan: "FREE" | "PAID";
};

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
  hello1(ctx: Context, req: Hello1Request): Promise<Hello1Response> {
    return this.request(ctx, "hello1", req);
  }
}
