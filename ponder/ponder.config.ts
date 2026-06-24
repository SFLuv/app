import { createConfig } from "ponder";
import { SFLUVv2ABI } from "./abis/SFLUVv2ABI";
import { erc20ABI } from "./abis/erc20ABI";

const startBlockEnv = process.env.PONDER_START_BLOCK;
const startBlockParsed = startBlockEnv ? Number(startBlockEnv) : 7650479;
const startBlock = Number.isFinite(startBlockParsed) ? startBlockParsed : 7650479;

export default createConfig({
  chains: {
    berachain: {
      id: 80094,
      rpc: process.env.PONDER_RPC_URL_1,
    },
  },
  contracts: {
    ERC20: {
      chain: "berachain",
      abi: erc20ABI,
      address: "0x881cad4f885c6701d8481c0ed347f6d35444ea7e",
      startBlock
    },
  },
});
