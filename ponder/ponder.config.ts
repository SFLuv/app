import { createConfig } from "ponder";
import { erc20ABI } from "./abis/erc20ABI";

const startBlock = Number(process.env.PONDER_START_BLOCK ?? 70348035);

export default createConfig({
  chains: {
    celo: {
      id: 42220,
      rpc: process.env.PONDER_RPC_URL_1,
    },
  },
  contracts: {
    ERC20: {
      chain: "celo",
      abi: erc20ABI,
      address: "0x881cad4f885c6701d8481c0ed347f6d35444ea7e",
      startBlock,
    },
  },
});