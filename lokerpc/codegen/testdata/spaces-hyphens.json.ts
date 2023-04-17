import { RPCContextClient } from "@loke/http-rpc-client";
import { Context } from "@loke/context";

export type User = {
  "\"DoubleQuotes\"": string;
  "#prefixed": string;
  $prefixed: string;
  "'SingleQuotes'": string;
  ".prefixed": string;
  "1numberedStart": string;
  "@prefixed": string;
  "Kebab-Case": string;
  PascalCase: string;
  Snake_Case: string;
  camelCase: string;
  "comma,separated": string;
  "dot.separated": string;
  "email@address": string;
  "kebab-case": string;
  lowercase: string;
  numberedEnd1: string;
  "semicolonsuffixed;": string;
  snake_case: string;
};

export type GetUserRequest = {
  id: string;
};

/**
 * 
 */
export class HyphenatedServiceNameService extends RPCContextClient {
  constructor(baseUrl: string) {
    super(baseUrl, "hyphenated-service-name")
  }
  /**
   * hello1 method
   */
  getUser(ctx: Context, req: GetUserRequest): Promise<User> {
    return this.request(ctx, "getUser", req);
  }
}
