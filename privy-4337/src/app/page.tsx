"use client"

import Image from "next/image";
import styles from "./page.module.css";
import useTestRequest from "@/hooks/useTestRequest";
import { usePrivy, useWallets } from "@privy-io/react-auth";
import { useEffect, useState } from "react";
import { Account, Address, createWalletClient, custom, Hex } from "viem";
import { polygon } from "viem/chains";
import { entryPoint07Abi, entryPoint07Address, toSmartAccount } from "viem/account-abstraction";
import { toSimpleSmartAccount } from "permissionless/accounts";
import { useApp } from "@/providers/AppProvider";

export default function Home() {
  const { isLoading, sendRequest, requestSent, requestSuccessful } = useTestRequest();
  const { login, logout, ready, authenticated, send, wallet } = useApp()
  const [answer, setAnswer] = useState("")
  const [gameActive, setGameActive] = useState(false)
  const [message, setMessage] = useState("")
  const { wallets } = useWallets()

  const runTest = async () => {
    const address = await wallet?.getAddress()
    console.log(address)
    if(address && (address != "" as Address)) {
      const res = await send(1n, address)
      if(res == null) {
        console.error("tx not sent")
        return
      }
      if(res?.error) {
        console.error(res.error)
        return
      }
      console.log(res?.hash)
    }
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
