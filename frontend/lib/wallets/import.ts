import { computeAddress, Wallet } from "ethers"
import { Address } from "viem"
import { encryptWithHpke } from "./encrypt"
import { createContext, ReactNode, useContext } from "react"

type ChainType = "ethereum"
type EntropyType = "private-key"
type EncryptionType = "HPKE"

const IMPORT_BASE_URL = "https://api.privy.io/v1/wallets/import"

interface BeginFlowReq {
  address: Address
  chain_type: ChainType
  entropy_type: EntropyType
  encryption_type: EncryptionType
}

interface BeginFlowRes {
  encryption_public_key: string
  encryption_type: EncryptionType
}

interface SubmissionWallet extends BeginFlowReq {
  ciphertext: string
  encapsulated_key: string
}

interface SubmissionReq {
  wallet: SubmissionWallet
  owner?: Object
  policy_ids?: Array<string>
  additional_signers?: Array<string>
}


export const importWallet = async (privateKey: string, authToken: string): Promise<string> => {
  const privy_id = process.env.NEXT_PUBLIC_PRIVY_APP_ID
  if(!privy_id) throw new Error("no privy id specified in .env")

  const pubKey = computeAddress(privateKey) as Address

  const headers: HeadersInit = {
    "Access-Token": authToken,
    "Content-Type": "application/json",
    "privy-app-id": privy_id
  }

  const initBody: BeginFlowReq = {
    address: pubKey,
    chain_type: "ethereum",
    entropy_type: "private-key",
    encryption_type: "HPKE"
  }

  let res = await fetch(IMPORT_BASE_URL + "/init", {
    method: "POST",
    body: JSON.stringify(initBody),
    headers
  })
  if(!res.ok) {
    let body = await res.text()
    console.error("res body: " + body)
    console.error(res)
    throw new Error("import init flow failed")
  }

  let initRes: BeginFlowRes = await res.json()
  let encrypted = await encryptWithHpke(initRes.encryption_public_key, privateKey)

  const subWallet: SubmissionWallet = {
    ...initBody,
    ciphertext: encrypted.ciphertext.toString(),
    encapsulated_key: encrypted.encapsulatedKey.toString()
  }
  const subBody: SubmissionReq = {
    wallet: subWallet
  }
  res = await fetch(IMPORT_BASE_URL + "/submit", {
    method: "POST",
    body: JSON.stringify(subBody),
    headers
  })
  if(!res.ok) {
    let body = await res.text()
    console.error("res body: " + body)
    console.error(res)
    throw new Error("import submit flow failed")
  }

  return pubKey
}
