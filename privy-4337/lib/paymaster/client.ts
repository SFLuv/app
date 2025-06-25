import { COMMUNITY } from "@lib/constants";
import { CHAIN } from "@lib/constants";
import { createPublicClient, http } from "viem";
import { createBundlerClient } from "viem/account-abstraction";

export const bundler = createBundlerClient({
  chain: CHAIN,
  transport: http(COMMUNITY.primaryRPCUrl)
})

export const client = createPublicClient({
  chain: CHAIN,
  transport: http(COMMUNITY.primaryRPCUrl)
})