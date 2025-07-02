import { AbiFunction } from "viem";

export const depositFor: AbiFunction = {
  "type": "function",
  "name": "depositFor",
  "inputs": [
    {
      "name": "account",
      "type": "address",
      "internalType": "address"
    },
    {
      "name": "amount",
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