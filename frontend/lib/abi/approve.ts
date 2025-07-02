import { AbiFunction } from "viem"

export const approve: AbiFunction = {
  "type": "function",
  "name": "approve",
  "inputs": [
    {
      "name": "spender",
      "type": "address",
      "internalType": "address"
    },
    {
      "name": "value",
      "type": "uint256",
      "internalType": "uint256"
    }
  ],
  "outputs": [
    {
      "name": "",
      "type": "bool",
      "internalType": "bool"
    }
  ],
  "stateMutability": "nonpayable"
}