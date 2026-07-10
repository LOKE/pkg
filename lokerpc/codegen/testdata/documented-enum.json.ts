import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

/**
 * Currency is a supported payment currency.
 */
export type Currency = "AUD" | "NZD";

/**
 * Payment describes a payment.
 */
export type Payment = {
  /**
   * Currency is used for the payment.
   */
  currency: Currency;
};

/**
 * PaymentsService manages payments.
 */
export class PaymentsService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "payments")
  }
  /**
   * CreatePayment creates a payment.
   */
  createPayment(ctx: Context, req: Payment): Promise<Payment> {
    return this.request(ctx, "createPayment", req);
  }
}
