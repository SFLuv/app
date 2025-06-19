import { useState } from "react";
import { usePrivy } from "@privy-io/react-auth";

export default function useRequest() {
  const [requestSent, setRequestSent] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [requestSuccessful, setRequestSuccessful] = useState<boolean>(false);
  const { authenticated, getAccessToken } = usePrivy();

  let baseUrl: string = process.env.NEXT_PUBLIC_BACKEND_BASE_URL || "http://localhost:8080"

  const sendRequest = async (endpoint: string, options: RequestInit) => {
    try {
      if(!authenticated) {
        throw new Error("user not signed in")
      }
      const accessToken = await getAccessToken() || ""
      if (accessToken == "") {
        throw new Error("error fetching accessToken")
      }
      setIsLoading(true)
      setRequestSent(true)

      const headers: HeadersInit = {
        "Access-Token": accessToken,
        ...options.headers
      }
      options.headers = headers
      const res = await fetch(baseUrl + endpoint, options)
      if (!res.ok) {
        throw new Error("fetch failed")
      }
      setRequestSuccessful(true)
    } catch (err) {
      setRequestSuccessful(false)
      console.error(err)
    } finally {
      setIsLoading(false)
    }
  }

  return {
    sendRequest,
    requestSent,
    isLoading,
    requestSuccessful
  }
}