import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type AddActivityRequest = {
  customerUid: string;
  item: 
| {
  amount: number;
  creditId: string;
  title: string;
  type: "CREDIT_EXPIRED";
}
| {
  body: string;
  title: string;
  type: "MESSAGE";
  locationName?: string;
}
| {
  points: number;
  title: string;
  type: "POINTS";
  locationName?: string;
};
};

export type AddActivityResponse = 
| {
  amount: number;
  creditId: string;
  id: string;
  timestamp: string;
  title: string;
  type: "CREDIT_EXPIRED";
}
| {
  body: string;
  id: string;
  timestamp: string;
  title: string;
  type: "MESSAGE";
  locationName?: string;
}
| {
  id: string;
  points: number;
  pointsText: string;
  timestamp: string;
  title: string;
  type: "POINTS";
  locationName?: string;
};

/**
 * customer activity feed
 */
export class ActivityService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "activity")
  }
  /**
   * Add an item to a customer's activity feed
   */
  addActivity(ctx: Context, req: AddActivityRequest): Promise<AddActivityResponse> {
    return this.request(ctx, "addActivity", req);
  }
}
