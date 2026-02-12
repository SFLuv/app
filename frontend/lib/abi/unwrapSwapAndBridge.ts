import { AbiFunction } from "viem";

export const unwrapSwapAndBridge: AbiFunction =  {
      "type": "function",
      "name": "unwrapSwapAndBridge",
      "inputs": [
        {
          "name": "amount",
          "type": "uint256",
          "internalType": "uint256"
        },
        {
          "name": "to",
          "type": "address",
          "internalType": "address"
        }
      ],
      "outputs": [
        {
          "name": "",
          "type": "tuple",
          "internalType": "struct MessagingReceipt",
          "components": [
            {
              "name": "guid",
              "type": "bytes32",
              "internalType": "bytes32"
            },
            {
              "name": "nonce",
              "type": "uint64",
              "internalType": "uint64"
            },
            {
              "name": "fee",
              "type": "tuple",
              "internalType": "struct MessagingFee",
              "components": [
                {
                  "name": "nativeFee",
                  "type": "uint256",
                  "internalType": "uint256"
                },
                {
                  "name": "lzTokenFee",
                  "type": "uint256",
                  "internalType": "uint256"
                }
              ]
            }
          ]
        },
        {
          "name": "",
          "type": "tuple",
          "internalType": "struct OFTReceipt",
          "components": [
            {
              "name": "amountSentLD",
              "type": "uint256",
              "internalType": "uint256"
            },
            {
              "name": "amountReceivedLD",
              "type": "uint256",
              "internalType": "uint256"
            }
          ]
        }
      ],
      "stateMutability": "nonpayable"
    }
