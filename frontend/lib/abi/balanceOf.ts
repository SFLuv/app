import { AbiFunction } from "viem";

export const balanceOf: AbiFunction = {
  "type": "function",
  "name": "balanceOf",
  "inputs": [
    {
      "name": "account",
      "type": "address",
      "internalType": "address"
    }
  ],
  "outputs": [
    {
      "name": "",
      "type": "uint256",
      "internalType": "uint256"
    }
  ],
  "stateMutability": "view"
}