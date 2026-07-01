import { createConfig } from "ponder";
import { erc20ABI } from "./abis/erc20ABI";

// Celo continuity ledger: the Berachain history is backfilled into this same
// database, and this instance indexes Celo forward from the migration's
// resolved Ponder start block. Override PONDER_START_BLOCK in the environment
// with `ponder_start_block` from the run's migration-result.json.
const startBlockEnv = process.env.PONDER_START_BLOCK;
const startBlockParsed = startBlockEnv ? Number(startBlockEnv) : 70956824;
const startBlock = Number.isFinite(startBlockParsed) ? startBlockParsed : 70956824;

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
