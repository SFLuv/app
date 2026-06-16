import { BundlerService } from "@citizenwallet/sdk";
import { createPublicClient, http } from "viem";
import { createBundlerClient } from "viem/account-abstraction";
import type { ResolvedCommunityConfig } from "@/lib/community-config";

export function createViemClients(config: ResolvedCommunityConfig) {
  const bundler = createBundlerClient({
    chain: config.chain,
    transport: http(config.rpcUrl, {
      methods: {
        include: ["pm_sponsorUserOperation", "eth_sendUserOperation"]
      }
    })
  })

  const cw_bundler = new BundlerService(config.community)

  const client = createPublicClient({
    chain: config.chain,
    transport: http(config.rpcUrl)
  })

  return { bundler, cw_bundler, client }
}
