import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type KountaMeta = {
  acceptedMessage: string;
  completedMessage: string;
  deliveryCompletedMessage: string;
  paymentMethods: {
  id: number;
  name: string;
}[];
  rejectedMessage: string;
  sites: {
  id: number;
  name: string;
}[];
  type: "native_kounta";
};

export type MetadataOnlyUnion = KountaMeta | ZonalMeta;

export type NormalDiscriminator = 
| {
  barValue: number;
  kind: "bar";
}
| {
  fooValue: string;
  kind: "foo";
};

export type OrderingConfigMeta = KountaMeta | PublicApiMeta | ZonalMeta;

export type PublicApiMeta = {
  type: "public_api";
};

export type ZonalMeta = {
  sites: {
  id: number;
  name: string;
  salesAreas: {
  id: number;
  name: string;
}[];
}[];
  type: "native_zonal";
};

export type GetConfigRequest = {
};

export type GetNormalRequest = {
};

export type GetMetadataOnlyRequest = {
};

/**
 * ordering service
 */
export class OrderingService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "ordering")
  }
  /**
   * get config
   */
  getConfig(ctx: Context, req: GetConfigRequest): Promise<OrderingConfigMeta> {
    return this.request(ctx, "getConfig", req);
  }
  /**
   * get normal discriminator
   */
  getNormal(ctx: Context, req: GetNormalRequest): Promise<NormalDiscriminator> {
    return this.request(ctx, "getNormal", req);
  }
  /**
   * get metadata only union
   */
  getMetadataOnly(ctx: Context, req: GetMetadataOnlyRequest): Promise<MetadataOnlyUnion> {
    return this.request(ctx, "getMetadataOnly", req);
  }
}
