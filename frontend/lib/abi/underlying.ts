import { AbiFunction } from "viem";

export const underlying: AbiFunction = {
  "type": "function",
  "name": "underlying",
  "inputs": [],
  "outputs": [
    {
      "name": "",
      "type": "address",
      "internalType": "contract IERC20"
    }
  ],
  "stateMutability": "view"
}
