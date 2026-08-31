import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type Customer = {
  customerId: string;
  uid: string;
  coords?: {
  lat: number;
  lng: number;
};
  guest?: boolean;
  name?: string;
  nullableTags?: string[] | null;
  scores?: Record<string, number>;
  seenAt?: string;
  tags?: string[];
};

/**
 * Optional properties: omitzero for collections and timestamps, omitempty otherwise
 */
export class DiscountsService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "discounts")
  }
  /**
   * Lock a discount for a customer
   */
  lockDiscount(ctx: Context, req: Customer): Promise<Customer> {
    return this.request(ctx, "lockDiscount", req);
  }
}
