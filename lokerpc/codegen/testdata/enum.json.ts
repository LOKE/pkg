import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type Currency = "AUD" | "NZD" | "USD" | "GBP" | "SGD";

export type PaymentState = "created" | "authorized" | "submitted" | "settled" | "failed" | "canceled" | "refunded" | "completed";

export type GetStateRequest = {
  amount: number;
  currency: Currency;
};

/**
 * Payment service
 */
export class PaymentService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "payment")
  }
  /**
   * Get payment state
   */
  getState(ctx: Context, req: GetStateRequest): Promise<PaymentState> {
    return this.request(ctx, "getState", req);
  }
}
