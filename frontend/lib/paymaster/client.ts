import { BundlerService } from "@citizenwallet/sdk";
import { createPublicClient, http } from "viem";
import { createBundlerClient } from "viem/account-abstraction";
import type { ResolvedCommunityConfig } from "@/lib/community-config";

export function createViemClients(config: ResolvedCommunityConfig) {
  const bundler = createBundlerClient({
    chain: config.chain,
    // AA bundler/paymaster methods are served only by the CW engine.
    transport: http(config.bundlerRpcUrl, {
      methods: {
        include: ["pm_sponsorUserOperation", "eth_sendUserOperation"]
      }
    })
  })

  const cw_bundler = new BundlerService(config.community)

  // Reads (eth_getCode/eth_getBalance/...) use a full node RPC; the engine 404s
  // those methods.
  const client = createPublicClient({
    chain: config.chain,
    transport: http(config.rpcUrl)
  })

  return { bundler, cw_bundler, client }
}
