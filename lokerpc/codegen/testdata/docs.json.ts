import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

/**
 * GetUserRequest is the request type for GetUser.
 */
export type GetUserRequest = {
  /** The unique identifier of the user. */
  id: string;
};

/**
 * User represents a user account.
 */
export type User = {
  id: string;
  /** The display name of the user. */
  name: string;
  /** Email address. Optional. */
  email?: string;
};

/**
 * A service for testing doc comments
 */
export class DocsTestService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "docs-test")
  }
  /**
   * Get a user by ID
   */
  getUser(ctx: Context, req: GetUserRequest): Promise<User> {
    return this.request(ctx, "getUser", req);
  }
}
