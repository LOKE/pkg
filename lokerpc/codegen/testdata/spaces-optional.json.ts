import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type AccountMetadata = {
  Environment: string;
  "Operator URL": string | null;
  "Organization ID": string;
  "Organization Name": string;
  "Location ID"?: string | null;
  "Location Name"?: string | null;
};

/**
 * Test service for AccountMetadata shape
 */
export class StripePaymentsService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "stripe-payments")
  }
  /**
   * Fetch account metadata
   */
  getAccountMetadata(ctx: Context, req: AccountMetadata): Promise<AccountMetadata> {
    return this.request(ctx, "getAccountMetadata", req);
  }
}
