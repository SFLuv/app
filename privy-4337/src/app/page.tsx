"use client"

import Image from "next/image";
import styles from "./page.module.css";
import useTestRequest from "@/hooks/useTestRequest";
import { usePrivy, useWallets } from "@privy-io/react-auth";
import { useEffect, useState } from "react";
import { Account, createWalletClient, custom, Hex } from "viem";
import { polygon } from "viem/chains";
import { entryPoint07Abi, entryPoint07Address, toSmartAccount } from "viem/account-abstraction";
import { toSimpleSmartAccount } from "permissionless/accounts";

export default function Home() {
  const { isLoading, sendRequest, requestSent, requestSuccessful } = useTestRequest();
  const { login, logout, ready, authenticated } = usePrivy()
  const [answer, setAnswer] = useState("")
  const [gameActive, setGameActive] = useState(false)
  const [message, setMessage] = useState("")
  const { wallets } = useWallets()

  const runTest = async () => {
    const wallet = wallets[0]

    console.log("wallet:", wallet)
    await wallet.switchChain(polygon.id)
    const provider = await wallet.getEthereumProvider()
    const client = createWalletClient({
      account: wallet.address as Hex,
      chain: polygon,
      transport: custom(provider),
    })

    // hard coded citizenwallet factory address for now, increment index for _nonce field in factory.
    const simpleSmartAccount = await toSimpleSmartAccount({
      owner: client,
      client,
      entryPoint: {
        address: entryPoint07Address,
        version: "0.7"
      },
      index: 1n,
      factoryAddress: "0x940Cbb155161dc0C4aade27a4826a16Ed8ca0cb2",
    })

    console.log("nonce:", await simpleSmartAccount.getNonce())
    console.log("factoryData:", await simpleSmartAccount.getFactoryArgs())

    const address = await simpleSmartAccount.getAddress()
    const sig = await simpleSmartAccount.signUserOperation({
      chainId: polygon.id,
      callData: "0x",
      callGasLimit: 0n,
      maxFeePerGas: 0n,
      maxPriorityFeePerGas: 0n,
      nonce: 0n,
      preVerificationGas: 0n,
      sender: "0x9e25Fe3734338F2cBF23e765a892a61AD23D19b2",
      signature: "0x",
      verificationGasLimit: 0n
    })

    console.log("address:", address)
    console.log("sig:", sig)
  }

  useEffect(() => {
    if(!authenticated && ready) {
      login()
    }
  }, [ready, authenticated])

  return (
    <>
    <div style={{textAlign: "center"}}>
      <div style={{display: "block", padding: "5px"}}>
        <button onClick={() => runTest()} disabled={isLoading || !authenticated}>
          {isLoading ? "Loading..." : "Test"}
        </button>
        {authenticated && <div style={{display: "block", padding: "5px"}}>
          <button onClick={logout}>
            Log Out
          </button>
        </div>}
      </div>
    </div>
    </>
  );
}
