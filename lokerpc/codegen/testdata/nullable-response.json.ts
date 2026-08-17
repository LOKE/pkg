import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type GetUserRequest = {
  name: string;
};

export type GetUserResponse = {
  name: string;
};

export type NullableGadget = {
  id: string;
} | null;

export type Widget = {
  id: string;
};

export type GetNicknameResponse = string | null;

export type GetTagsResponse = string[] | null;

export type GetUserRequest_ = {
  id: string;
};

export type GetUserResponse_ = {
  id: string;
} | null;

/**
 * 
 */
export class TypedService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "typed")
  }
  /**
   * ref (not itself marked nullable) pointing at a definition that is nullable
   */
  getGadget(ctx: Context, req: any): Promise<NullableGadget> {
    return this.request(ctx, "getGadget", req);
  }
  /**
   * nullable scalar response
   */
  getNickname(ctx: Context, req: any): Promise<GetNicknameResponse> {
    return this.request(ctx, "getNickname", req);
  }
  /**
   * nullable array response
   */
  getTags(ctx: Context, req: any): Promise<GetTagsResponse> {
    return this.request(ctx, "getTags", req);
  }
  /**
   * inline nullable with properties
   */
  getUser(ctx: Context, req: GetUserRequest_): Promise<GetUserResponse_> {
    return this.request(ctx, "getUser", req);
  }
  /**
   * ref with nullable set on the ref usage itself, pointing at a non-nullable definition
   */
  getWidget(ctx: Context, req: any): Promise<Widget | null> {
    return this.request(ctx, "getWidget", req);
  }
}
