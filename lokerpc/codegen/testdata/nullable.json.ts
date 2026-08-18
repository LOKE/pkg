import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type NullableGadget = {
  id: string;
} | null;

export type Widget = {
  id: string;
};

export type GetNicknameResponse = string | null;

export type GetTagsResponse = string[] | null;

export type GetUserRequest = {
  id: string;
};

export type GetUserResponse = {
  id: string;
} | null;

export type SetNicknameRequest = {
  id: string;
} | null;

export type SetTagsRequest = string[] | null;

export type SetTitleRequest = string | null;

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
  getUser(ctx: Context, req: GetUserRequest): Promise<GetUserResponse> {
    return this.request(ctx, "getUser", req);
  }
  /**
   * ref with nullable set on the ref usage itself, pointing at a non-nullable definition
   */
  getWidget(ctx: Context, req: any): Promise<Widget | null> {
    return this.request(ctx, "getWidget", req);
  }
  /**
   * ref (not itself marked nullable) pointing at a definition that is nullable (request side)
   */
  setGadget(ctx: Context, req: NullableGadget): Promise<void> {
    return this.request(ctx, "setGadget", req);
  }
  /**
   * nullable inline request
   */
  setNickname(ctx: Context, req: SetNicknameRequest): Promise<void> {
    return this.request(ctx, "setNickname", req);
  }
  /**
   * nullable array request
   */
  setTags(ctx: Context, req: SetTagsRequest): Promise<void> {
    return this.request(ctx, "setTags", req);
  }
  /**
   * nullable scalar request
   */
  setTitle(ctx: Context, req: SetTitleRequest): Promise<void> {
    return this.request(ctx, "setTitle", req);
  }
  /**
   * ref with nullable set on the ref usage itself, pointing at a non-nullable definition (request side)
   */
  setWidget(ctx: Context, req: Widget | null): Promise<void> {
    return this.request(ctx, "setWidget", req);
  }
}
