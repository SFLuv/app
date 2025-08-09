import { AbiFunction } from "viem";

export const transfer: AbiFunction = {
  "type": "function",
  "name": "transfer",
  "inputs": [
    {
      "name": "to",
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