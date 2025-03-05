"use client"

import CircularProgress from '@mui/material/CircularProgress';
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { verifyAccountOwnership } from "@citizenwallet/sdk";
import "./redeem.css";


const Page = () => {
  const searchParams = useSearchParams();
  const router = useRouter();

  const [error, setError] = useState();
  const [success, setSuccess] = useState();
  const [accountLinked, setAccountLinked] = useState(false);
  const [inputting, setInputting] = useState(false);

  const [email, setEmail] = useState("");
  const [name, setName] = useState("");

  const sigAuthAccount = searchParams.get("sigAuthAccount")
  const sigAuthSignature = searchParams.get("sigAuthSignature")
  const sigAuthRedirect = searchParams.get("sigAuthRedirect")
  const sigAuthExpiry = searchParams.get("sigAuthExpiry")
  const code = searchParams.get("code")


  const closeModal = (delay) => {
    setTimeout(() => {
        if (sigAuthRedirect) {
          router.push(sigAuthRedirect + "/close")
        }
      }, [delay])
  }

  useEffect(() => {
    if (!sigAuthAccount || !sigAuthSignature || !code) {
      setError("Please download the CitizenWallet app, then scan your QR code again.")
      return
    }
    getAccountLinked()
  }, [])

  useEffect(() => {
    console.log('sending')
    if (accountLinked) {
      sendBotRequest()
    }
  }, [accountLinked])

  const sendBotRequest = async () => {

    // let verified = verifyAccountOwnership()
    //implement real verification
    try {
          let res = await fetch(process.env.NEXT_PUBLIC_BACKEND_SERVER + "/redeem", {
            method: "POST",
            body: JSON.stringify({
              code,
              address: sigAuthAccount
            })
          });

          if (res.status != 200) {
            let text = await res.text()
            switch (text) {
              case "code expired":
                setError("Code expired.")
                break;
              case "code redeemed":
                setError("Code already redeemed.")
                break;
              case "user redeemed":
                setError("User already redeemed for this event.")
                break;
              default:
                setError("Error redeeming code.")
            }
            closeModal(2000)
            return
          }

          setSuccess(true)
          closeModal(2000)
    } catch {
      setError("Internal server error.")
      closeModal(2000)
      return
    }

    //redirect back to app
  }

  const getAccountLinked = async () => {
    try {
      let res = await fetch(process.env.NEXT_PUBLIC_BACKEND_SERVER + "/account?address="
        + sigAuthAccount
      );

      if (res.status != 200) {
        setAccountLinked(true)
        return
      }

      let body = await res.json()
      if (body?.account === true) {
        setAccountLinked(true)
        return
      }


      setInputting(true)
    } catch {
      setAccountLinked(true)
      return
    }
  }

  const sendAccountLink = async () => {
    try {
      let res = await fetch(process.env.NEXT_PUBLIC_BACKEND_SERVER + "/account", {
        method: "POST",
        body: JSON.stringify({
          address: sigAuthAccount,
          email,
          name
        })
      });


    } catch {
      setAccountLinked(true)
      return
    }

    setAccountLinked(true)
    return
  }

  const labelStyle = {
    marginTop: "1vh",
    color: "black",
    fontWeight: "bold",
    fontFamily: "Arial"
  }

  const inputStyle = {
    margin: "5px",
    marginTop: "1vh",
    width: "calc(100% - 20px)",
    height: "50px",
    padding: "5px",
    border: "none",
    borderRadius: "5px",
    textAlign: "center",
    fontFamily: "Arial",
    fontSize: "22px",
    "&:active": {
      border: "none",
      outline: "none"
    },
    "&:focusVisible": {
      outline: "none"
    },
    "&:focus": {
      outline: "none"
    },
  }

  const buttonStyle = {
    margin: "auto",
    cursor: "pointer",
    marginTop: "2vh",
    width: "100px",
    height: "50px",
    border: "none",
    borderRadius: "5px",
    color: "whitesmoke",
    backgroundColor: "#eb6c6c",
    fontFamily: "Arial",
    fontWeight: "bold",
    fontSize: "18px"
  }

  const secondaryButtonStyle = {
    backgroundColor: "transparent",
    cursor: "pointer",
    margin: "auto",
    marginTop: "2vh",
    width: "40px",
    color: "#eb6c6c",
    border: "none",
    fontSize: "14px"
  }

  const titleStyle = {
    textAlign: "center",
    color: "black",
    fontFamily: "Arial"
  }



  return (
    <div style={{ display: "flex", height: "100vh", width: "100vw", color: "#eb6c6c" }}>
      <div style={{ display: "flex", margin: "auto" }}>
      {
          error ?
          <div style={{textAlign: "center"}}>
            <h2 style={{color: "black", size: "4vh"}}>
              {error}
            </h2>
            {error === "Please download the CitizenWallet app, then scan your QR code again." && <>
              <a href="https://apps.apple.com/us/app/citizen-wallet/id6460822891">
                <img
                  style={{maxHeight: "20vh", maxWidth: "30vw", cursor: "pointer"}}
                  src="/appstore.svg"
                  />
              </a>
              <a href="https://play.google.com/store/apps/details?id=xyz.citizenwallet.wallet&hl=en&pli=1">
                <img
                  style={{maxHeight: "20vh", maxWidth: "30vw", cursor: "pointer"}}
                  src="/googleplaystore.svg"
                  />
              </a>
            </>}
          </div>
          : success ?
          <div style={{textAlign: "center"}}>
            <h2 style={{color: "black", size: "4vh"}}>
              Code redeemed!
            </h2>
          </div>
          : inputting ?
          <div style={{width: "70vw", marginBottom: "10vh"}}>
            <h1 style={titleStyle}>
              Want to hear about future SFLuv events?
            </h1>
            <form
              style={{
                display: "grid",
                gridTemplateColumns: "1fr",
                width: "100%"
              }}
              onSubmit={(e) => {
                e.preventDefault()
                sendAccountLink()
              }}
            >
              <label style={labelStyle}>Email:</label>
              <input
                style={inputStyle}
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value)
                }}
              />
              <label style={labelStyle}>Name:</label>
              <input
                style={inputStyle}
                value={name}
                onChange={(e) => {
                  setName(e.target.value)
                }}
              />
              <button style={buttonStyle} type="submit">Submit</button>
              <button
                style={secondaryButtonStyle}
                type="button"
                onClick={() => {
                  setInputting(false)
                  setAccountLinked(true)
                }}
              >Skip</button>
            </form>
          </div>
          :
          <div style={{textAlign: "center"}}>
            <h2 style={{color: "black", size: "4vh"}}>Redeeming...</h2>
            <CircularProgress color="inherit" size="8vh"/>
          </div>
        }
      </div>
    </div>
  )
}

export default Page;