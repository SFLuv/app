import { chain, paymasterUrl } from "@config";
import { http } from "viem";
import { createBundlerClient } from "viem/account-abstraction";

export const client = createBundlerClient({
  chain,
  transport: http(paymasterUrl)
})