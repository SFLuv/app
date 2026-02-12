import { AbiFunction } from "viem"

export const allowance: AbiFunction = {
  "type": "function",
  "name": "allowance",
  "inputs": [
    {
      "name": "owner",
      "type": "address",
      "internalType": "address"
    },
    {
      "name": "spender",
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
