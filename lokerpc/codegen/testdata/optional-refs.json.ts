import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type Coords = {
  lat: number;
  lng: number;
};

export type Count = number;

export type CountAlias = Count;

export type CreatedAt = string;

export type CreatedAtAlias = CreatedAt;

export type Filter = {
  coords?: Coords;
  count?: Count;
  countAlias?: CountAlias;
  createdAt?: CreatedAt;
  createdAtAlias?: CreatedAtAlias;
  inlineCount?: number;
  maybeCount?: MaybeCount;
  ratio?: Ratio;
  tags?: Tags;
};

export type MaybeCount = number | null;

export type Ratio = number;

export type Tags = string[];

export type SearchResponse = Count[];

/**
 * 
 */
export class OptionalRefsService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "optional-refs")
  }
  /**
   * search method
   */
  search(ctx: Context, req: Filter): Promise<SearchResponse> {
    return this.request(ctx, "search", req);
  }
}
