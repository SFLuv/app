"use client"

import Link from "next/link"
import { useMemo, useState } from "react"
import { useApp } from "@/context/AppProvider"
import type { AccountDeletionStatusResponse } from "@/types/server"

const SUPPORT_EMAIL = "techsupport@sfluv.org"
const WORDPRESS_HELP_URL = "https://sfluv.org/delete-account#sfluv-delete-help"
const POLICY_REQUIRED_HEADER = "X-SFLUV-Auth-Reason"
const POLICY_REQUIRED_REASON = "privacy-policy-required"

type SubmitPhase = "idle" | "submitting" | "success"

const supportMailto =
  `mailto:${SUPPORT_EMAIL}?subject=${encodeURIComponent("SFLuv account deletion request")}`

const formatDeleteDate = (value?: string | null) => {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""

  return new Intl.DateTimeFormat("en-US", {
    month: "long",
    day: "numeric",
    year: "numeric",
  }).format(date)
}

const readResponseText = async (res: Response) => {
  try {
    return (await res.text()).trim()
  } catch {
    return ""
  }
}

const readDeletionStatus = async (
  res: Response,
): Promise<AccountDeletionStatusResponse | null> => {
  try {
    return (await res.json()) as AccountDeletionStatusResponse
  } catch {
    return null
  }
}

export default function DeleteAccountPage() {
  const { authFetch, login, logout, status, user } = useApp()
  const [phase, setPhase] = useState<SubmitPhase>("idle")
  const [loginBusy, setLoginBusy] = useState(false)
  const [confirmed, setConfirmed] = useState(false)
  const [error, setError] = useState("")
  const [deletionStatus, setDeletionStatus] =
    useState<AccountDeletionStatusResponse | null>(null)

  const deleteDateLabel = useMemo(
    () => formatDeleteDate(deletionStatus?.delete_date),
    [deletionStatus?.delete_date],
  )

  const handleLogin = async () => {
    setError("")
    setLoginBusy(true)
    try {
      await login()
    } catch (loginError) {
      setError(
        (loginError as Error)?.message?.trim() ||
          "We could not open secure sign-in. Please try again.",
      )
    } finally {
      setLoginBusy(false)
    }
  }

  const handleDeleteAccount = async () => {
    if (!confirmed || phase === "submitting") return

    setError("")
    setPhase("submitting")

    try {
      const res = await authFetch("/users/delete-account", { method: "POST" })

      if (res.status === 202) {
        setDeletionStatus(await readDeletionStatus(res))
        setPhase("success")
        try {
          await logout()
        } catch (logoutError) {
          console.error("Unable to sign out after account deletion", logoutError)
        }
        return
      }

      if (res.status === 409) {
        setPhase("success")
        try {
          await logout()
        } catch (logoutError) {
          console.error("Unable to sign out after account deletion", logoutError)
        }
        return
      }

      if (
        res.status === 403 &&
        res.headers.get(POLICY_REQUIRED_HEADER) === POLICY_REQUIRED_REASON
      ) {
        setError(
          "Please complete the policy prompt, then confirm deletion again.",
        )
        setPhase("idle")
        return
      }

      const text = await readResponseText(res)
      if (res.status === 404) {
        throw new Error(
          "We could not find a SFLuv account for this sign-in. Use the support request option below and we will help verify ownership.",
        )
      }
      throw new Error(text || "Unable to start account deletion right now.")
    } catch (deleteError) {
      setError(
        (deleteError as Error)?.message?.trim() ||
          "Unable to start account deletion right now.",
      )
      setPhase("idle")
    }
  }

  return (
    <main className="min-h-screen overflow-hidden bg-[#fff8f1] px-4 py-8 text-slate-950 sm:px-6 lg:px-8">
      <div className="pointer-events-none fixed inset-0 -z-10 bg-[radial-gradient(circle_at_20%_10%,rgba(235,108,108,0.20),transparent_34%),radial-gradient(circle_at_85%_12%,rgba(255,195,132,0.24),transparent_32%),linear-gradient(145deg,#fffaf4_0%,#f7efe5_100%)]" />

      <section className="mx-auto flex min-h-[calc(100vh-4rem)] max-w-3xl items-center">
        <div className="w-full rounded-[34px] border border-[#eb6c6c]/20 bg-white/95 p-6 shadow-[0_30px_90px_rgba(125,55,38,0.16)] backdrop-blur sm:p-10">
          <div className="mb-8">
            <p className="text-sm font-black uppercase tracking-[0.24em] text-[#eb6c6c]">
              SFLuv Account
            </p>
            <h1 className="mt-4 text-4xl font-black tracking-tight text-slate-950 sm:text-5xl">
              Delete your SFLuv account
            </h1>
            <p className="mt-4 max-w-2xl text-base leading-7 text-slate-600">
              Sign in with SFLuv, confirm the request, and we will deactivate
              your account and schedule eligible account data for deletion.
            </p>
          </div>

          {phase === "success" ? (
            <SuccessState deleteDateLabel={deleteDateLabel} />
          ) : status === "unauthenticated" ? (
            <SignInState
              busy={loginBusy}
              error={error}
              onLogin={() => {
                void handleLogin()
              }}
            />
          ) : status === "loading" ? (
            <LoadingState />
          ) : (
            <ConfirmState
              confirmed={confirmed}
              error={error}
              phase={phase}
              userEmail={user?.contact_email}
              userId={user?.id}
              onConfirmedChange={setConfirmed}
              onDelete={() => {
                void handleDeleteAccount()
              }}
            />
          )}

          <div className="mt-8 grid gap-3 rounded-3xl border border-slate-200 bg-[#fffaf8] p-5 text-sm leading-6 text-slate-700 sm:grid-cols-2">
            <div>
              <p className="font-bold text-slate-950">Can’t sign in?</p>
              <p className="mt-1">
                Use the support form on SFLuv.org or email us directly.
              </p>
              <div className="mt-3 flex flex-wrap gap-2">
                <Link className="font-bold text-[#b64b46] underline" href={WORDPRESS_HELP_URL}>
                  Open support form
                </Link>
                <a className="font-bold text-[#b64b46] underline" href={supportMailto}>
                  Email support
                </a>
              </div>
            </div>
            <div>
              <p className="font-bold text-slate-950">Recovery window</p>
              <p className="mt-1">
                Contact us within 30 days if you need to recover your account.
                Never send private keys, seed phrases, or passwords.
              </p>
            </div>
          </div>
        </div>
      </section>
    </main>
  )
}

function SignInState({
  busy,
  error,
  onLogin,
}: {
  busy: boolean
  error: string
  onLogin: () => void
}) {
  return (
    <div className="rounded-3xl border border-slate-200 bg-white p-5 sm:p-6">
      <h2 className="text-2xl font-black text-slate-950">
        Start with secure sign-in
      </h2>
      <p className="mt-3 text-sm leading-6 text-slate-600">
        Use the same sign-in method you use for SFLuv. After sign-in, you’ll
        see one final confirmation before deletion is requested.
      </p>

      {error ? <ErrorMessage message={error} /> : null}

      <button
        type="button"
        disabled={busy}
        onClick={onLogin}
        className="mt-6 inline-flex w-full items-center justify-center rounded-full bg-[#eb6c6c] px-5 py-3 text-base font-black text-white shadow-[0_12px_30px_rgba(235,108,108,0.28)] transition hover:bg-[#d85b5b] disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto"
      >
        {busy ? "Opening sign-in..." : "Sign in to delete my account"}
      </button>
    </div>
  )
}

function LoadingState() {
  return (
    <div className="rounded-3xl border border-slate-200 bg-white p-6 text-center">
      <div className="mx-auto h-12 w-12 animate-spin rounded-full border-4 border-[#f4c1b7] border-t-[#eb6c6c]" />
      <p className="mt-4 font-bold text-slate-950">Preparing secure sign-in...</p>
      <p className="mt-2 text-sm text-slate-600">
        If a policy prompt appears, complete it before confirming deletion.
      </p>
    </div>
  )
}

function ConfirmState({
  confirmed,
  error,
  phase,
  userEmail,
  userId,
  onConfirmedChange,
  onDelete,
}: {
  confirmed: boolean
  error: string
  phase: SubmitPhase
  userEmail?: string
  userId?: string
  onConfirmedChange: (confirmed: boolean) => void
  onDelete: () => void
}) {
  const submitting = phase === "submitting"

  return (
    <div className="rounded-3xl border border-[#eb6c6c]/30 bg-[#fff7f4] p-5 sm:p-6">
      <h2 className="text-2xl font-black text-slate-950">
        Confirm account deletion
      </h2>
      <p className="mt-3 text-sm leading-6 text-slate-700">
        This will deactivate your SFLuv account and schedule eligible account
        data for deletion. You can contact SFLuv within 30 days if you need
        account recovery.
      </p>

      <div className="mt-5 rounded-2xl border border-slate-200 bg-white p-4 text-sm leading-6 text-slate-700">
        <p className="font-bold text-slate-950">Signed in account</p>
        <p className="mt-1 break-all">
          {userEmail || userId || "Authenticated SFLuv account"}
        </p>
      </div>

      <label className="mt-5 flex items-start gap-3 rounded-2xl border border-[#eb6c6c]/30 bg-white p-4 text-sm leading-6 text-slate-700">
        <input
          type="checkbox"
          checked={confirmed}
          disabled={submitting}
          onChange={(event) => onConfirmedChange(event.target.checked)}
          className="mt-1 h-4 w-4 accent-[#eb6c6c]"
        />
        <span>
          I request deletion of my SFLuv account and associated account data. I
          understand SFLuv may retain limited records where required for
          security, fraud prevention, legal, regulatory, or compliance reasons.
        </span>
      </label>

      {error ? <ErrorMessage message={error} /> : null}

      <button
        type="button"
        disabled={!confirmed || submitting}
        onClick={onDelete}
        className="mt-6 inline-flex w-full items-center justify-center rounded-full bg-[#eb6c6c] px-5 py-3 text-base font-black text-white shadow-[0_12px_30px_rgba(235,108,108,0.28)] transition hover:bg-[#d85b5b] disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto"
      >
        {submitting ? "Starting deletion..." : "Yes, delete my account"}
      </button>
    </div>
  )
}

function SuccessState({ deleteDateLabel }: { deleteDateLabel: string }) {
  return (
    <div className="rounded-3xl border border-emerald-200 bg-emerald-50 p-5 sm:p-6">
      <p className="text-sm font-black uppercase tracking-[0.22em] text-emerald-700">
        Request Started
      </p>
      <h2 className="mt-3 text-3xl font-black text-slate-950">
        Your account deletion process has been started.
      </h2>
      <p className="mt-3 text-sm leading-6 text-slate-700">
        Please contact us within 30 days if you would like to recover your
        account.
      </p>
      {deleteDateLabel ? (
        <p className="mt-3 rounded-2xl border border-emerald-200 bg-white/80 p-4 text-sm font-bold text-slate-800">
          Scheduled deletion date: {deleteDateLabel}
        </p>
      ) : null}
      <div className="mt-6">
        <a
          href={supportMailto}
          className="inline-flex w-full items-center justify-center rounded-full bg-emerald-700 px-5 py-3 text-base font-black text-white transition hover:bg-emerald-800 sm:w-auto"
        >
          Contact support
        </a>
      </div>
    </div>
  )
}

function ErrorMessage({ message }: { message: string }) {
  return (
    <p className="mt-4 rounded-2xl border border-red-200 bg-red-50 p-4 text-sm font-bold leading-6 text-red-800">
      {message}
    </p>
  )
}
