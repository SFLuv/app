import { BundlerService } from "@citizenwallet/sdk";
import config from "@config";
import { COMMUNITY } from "@lib/constants";
import { CHAIN } from "@lib/constants";
import { createPublicClient, http } from "viem";
import { createBundlerClient } from "viem/account-abstraction";

export const bundler = createBundlerClient({
  chain: CHAIN,
  transport: http(COMMUNITY.primaryRPCUrl, {
    methods: {
      include: ["pm_sponsorUserOperation", "eth_sendUserOperation"]
    }
  })
})

export const cw_bundler = new BundlerService(COMMUNITY)

export const client = createPublicClient({
  chain: CHAIN,
  transport: http(COMMUNITY.primaryRPCUrl)
})