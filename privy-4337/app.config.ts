import { polygon } from "viem/chains"

export const chain = polygon
const config = {
  "community": {
      "name": "SFLUV Community",
      "description": "A community currency for the city of San Francisco.",
      "url": "https://sfluv.org",
      "alias": "wallet.sfluv.org",
      "custom_domain": "wallet.sfluv.org",
      "logo": "https://assets.citizenwallet.xyz/wallet-config/_images/sfluv.svg",
      "theme": {
          "primary": "#eb6c6c"
      },
      "profile": {
          "address": "0x05e2Fb34b4548990F96B3ba422eA3EF49D5dAa99",
          "chain_id": 137
      },
      "primary_token": {
          "address": "0x58a2993A618Afee681DE23dECBCF535A58A080BA",
          "chain_id": 137
      },
      "primary_account_factory": {
          "address": "0x5e987a6c4bb4239d498E78c34e986acf29c81E8e",
          "chain_id": 137
      }
  },
  "tokens": {
      "137:0x58a2993A618Afee681DE23dECBCF535A58A080BA": {
          "standard": "erc20",
          "name": "SFLUV V1.1",
          "address": "0x58a2993A618Afee681DE23dECBCF535A58A080BA",
          "symbol": "SFLUV",
          "decimals": 6,
          "chain_id": 137
      }
  },
  "scan": {
      "url": "https://polygonscan.com",
      "name": "Polygon Explorer"
  },
  "accounts": {
      "137:0x5e987a6c4bb4239d498E78c34e986acf29c81E8e": {
          "chain_id": 137,
          "entrypoint_address": "0x2d01C5E40Aa6a8478eD0FFbF2784EBb9bf67C46A",
          "paymaster_address": "0x7FC98D0a2bd7f766bAca37388eB0F6Db37666B33",
          "account_factory_address": "0x5e987a6c4bb4239d498E78c34e986acf29c81E8e",
          "paymaster_type": "cw"
      }
  },
  "chains": {
      "137": {
          "id": 137,
          "node": {
              "url": "https://137.engine.citizenwallet.xyz",
              "ws_url": "wss://137.engine.citizenwallet.xyz"
          }
      }
  },
  "ipfs": {
      "url": "https://ipfs.internal.citizenwallet.xyz"
  },
  "plugins": [
      {
          "name": "About",
          "icon": "https://wallet.sfluv.org/uploads/logo.svg",
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
