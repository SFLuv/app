import { berachain } from "viem/chains"

export const chain = berachain
const config = {
        "community": {
            "name": "SFLUV Community",
            "description": "A community currency for the city of San Francisco.",
            "url": "https://sfluv.org",
            "alias": "wallet.berachain.sfluv.org",
            "custom_domain": "wallet.sfluv.org",
            "logo": "https://assets.citizenwallet.xyz/wallet-config/_images/sfluv.svg",
            "theme": {
                "primary": "#eb6c6c"
            },
            "profile": {
                "address": "0x05e2Fb34b4548990F96B3ba422eA3EF49D5dAa99",
                "chain_id": 80094
            },
            "primary_token": {
                "address": "0x881cad4f885c6701d8481c0ed347f6d35444ea7e",
                "chain_id": 80094
            },
            "primary_account_factory": {
                "address": "0x7cC54D54bBFc65d1f0af7ACee5e4042654AF8185",
                "chain_id": 80094
            }
        },
        "tokens": {
            "80094:0x881cad4f885c6701d8481c0ed347f6d35444ea7e": {
                "standard": "erc20",
                "name": "SFLUV V1.1",
                "address": "0x881cad4f885c6701d8481c0ed347f6d35444ea7e",
                "symbol": "SFLUV",
                "decimals": 18,
                "chain_id": 80094
            }
        },
        "scan": {
            "url": "https://polygonscan.com",
            "name": "Polygon Explorer"
        },
        "accounts": {
            "80094:0x7cC54D54bBFc65d1f0af7ACee5e4042654AF8185": {
                "chain_id": 80094,
                "entrypoint_address": "0x7079253c0358eF9Fd87E16488299Ef6e06F403B6",
                "paymaster_address": "0x9A5be02B65f9Aa00060cB8c951dAFaBAB9B860cd",
                "account_factory_address": "0x7cC54D54bBFc65d1f0af7ACee5e4042654AF8185",
                "paymaster_type": "cw-safe"
            }
        },
        "chains": {
            "80094": {
                "id": 80094,
                "node": {
                    "url": "https://80094.engine.citizenwallet.xyz",
                    "ws_url": "wss://80094.engine.citizenwallet.xyz"
                }
            }
        },
        "ipfs": {
            "url": "https://ipfs.internal.citizenwallet.xyz"
        },
        "plugins": [
            {
                "name": "About",
                "icon": "https://assets.citizenwallet.xyz/wallet-config/_images/sfluv.svg",
                "url": "https://app.sfluv.org",
                "launch_mode": "webview",
                "signature": true,
                "hidden": true
            }
        ],
        "config_location": "https://config.internal.citizenwallet.xyz/v4/wallet.sfluv.org.json",
        "version": 4
    }


export default config
