import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type Change = 
| {
  id: string;
  type: "CREATED";
};

export type ChangeAlias = Change;

export type Event = 
| {
  id: string;
  type: "CREATED";
}
| {
  id: string;
  type: "DELETED";
} | null;

export type EventsByID = Record<string, 
| {
  id: string;
  type: "CREATED";
}>;

export type ListResponse = (
| {
  id: string;
  type: "CREATED";
}
| {
  id: string;
  type: "DELETED";
})[];

export type PublishRequest = {
  id: string;
  event?: 
| {
  id: string;
  type: "CREATED";
};
};

/**
 * union shapes
 */
export class EventsService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "events")
  }
  /**
   * latest event
   */
  latest(ctx: Context, req: any): Promise<Event> {
    return this.request(ctx, "latest", req);
  }
  /**
   * list events
   */
  list(ctx: Context, req: any): Promise<ListResponse> {
    return this.request(ctx, "list", req);
  }
  /**
   * publish an event
   */
  publish(ctx: Context, req: PublishRequest): Promise<void> {
    return this.request(ctx, "publish", req);
  }
}
