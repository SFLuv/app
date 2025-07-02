import { AbiFunction } from "viem";

export const withdrawTo: AbiFunction = {
  "type": "function",
  "name": "withdrawTo",
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