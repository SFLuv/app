"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Workflow } from "@/types/workflow"
import { AlertTriangle, Clock3, Vote } from "lucide-react"

function countdownText(finalizeAt?: string | null): string {
  if (!finalizeAt) return "Countdown not started"
  const remainingMs = new Date(finalizeAt).getTime() - Date.now()
  if (remainingMs <= 0) return "Finalization pending"

  const hours = Math.floor(remainingMs / (1000 * 60 * 60))
  const minutes = Math.floor((remainingMs % (1000 * 60 * 60)) / (1000 * 60))
  return `${hours}h ${minutes}m remaining`
}

export default function VoterPage() {
  const { authFetch, status, user } = useApp()
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [loading, setLoading] = useState<boolean>(true)
  const [error, setError] = useState<string>("")
  const [submittingId, setSubmittingId] = useState<string>("")

  const canVote = Boolean(user?.isVoter || user?.isAdmin)

  const loadWorkflows = useCallback(async () => {
    if (!canVote) {
      setLoading(false)
      return
    }

    try {
      const res = await authFetch("/voters/workflows")
      if (!res.ok) throw new Error()
      const data = (await res.json()) as Workflow[]
      setWorkflows(data || [])
      setError("")
    } catch {
      setError("Unable to load workflows for voting right now.")
    } finally {
      setLoading(false)
    }
  }, [authFetch, canVote])

  useEffect(() => {
    if (status !== "authenticated") return
    loadWorkflows()
  }, [status, loadWorkflows])

  useEffect(() => {
    if (status !== "authenticated" || !canVote) return
    const interval = setInterval(() => {
      loadWorkflows()
    }, 30000)
    return () => clearInterval(interval)
  }, [status, canVote, loadWorkflows])

  const voteWorkflow = async (workflowId: string, decision: "approve" | "deny") => {
    setSubmittingId(workflowId + decision)
    try {
      const res = await authFetch(`/workflows/${workflowId}/votes`, {
        method: "POST",
        body: JSON.stringify({ decision }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to submit vote.")
      }
      await loadWorkflows()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to submit vote right now.")
    } finally {
      setSubmittingId("")
    }
  }

  const forceApprove = async (workflowId: string) => {
    setSubmittingId(workflowId + "force")
    try {
      const res = await authFetch(`/admin/workflows/${workflowId}/force-approve`, {
        method: "POST",
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to force approve workflow.")
      }
      await loadWorkflows()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to force approve workflow.")
    } finally {
      setSubmittingId("")
    }
  }

  const pendingCount = useMemo(() => workflows.filter((workflow) => workflow.status === "pending").length, [workflows])

  if (status === "loading" || loading) {
    return (
      <div className="flex items-center justify-center min-h-[70vh]">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  if (!canVote) {
    return (
      <div className="container mx-auto p-4 space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Voter Access Required</CardTitle>
            <CardDescription>
              Your account is not approved for voter access yet. Admins can grant this in the admin panel.
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-4 space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Voter Panel</h1>
        <p className="text-muted-foreground">Review workflow proposals and vote on approval.</p>
      </div>

      {error && (
        <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
          <AlertTriangle className="h-4 w-4 flex-shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Vote className="h-5 w-5" />
            Workflow Votes
          </CardTitle>
          <CardDescription>
            Pending proposals: <span className="font-medium">{pendingCount}</span>
          </CardDescription>
        </CardHeader>
        <CardContent>
          {workflows.length === 0 ? (
            <p className="text-sm text-muted-foreground">No workflows available for voting.</p>
          ) : (
            <div className="space-y-4">
              {workflows.map((workflow) => {
                const pending = workflow.status === "pending"
                const myDecision = workflow.votes.my_decision

                return (
                  <Card key={workflow.id}>
                    <CardContent className="p-4 space-y-3">
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <div>
                          <h3 className="font-semibold">{workflow.title}</h3>
                          <p className="text-xs text-muted-foreground">Workflow ID: {workflow.id}</p>
                        </div>
                        <Badge variant={pending ? "outline" : workflow.status === "approved" || workflow.status === "blocked" ? "default" : "secondary"}>
                          {workflow.status}
                        </Badge>
                      </div>

                      <p className="text-sm text-muted-foreground">{workflow.description}</p>

                      <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
                        <span>
                          Votes: {workflow.votes.approve} approve / {workflow.votes.deny} deny ({workflow.votes.votes_cast}/{workflow.votes.total_voters})
                        </span>
                        <span>Quorum threshold: {workflow.votes.quorum_threshold}</span>
                        <span>Start: {new Date(workflow.start_at).toLocaleString()}</span>
                      </div>

                      {pending && workflow.votes.quorum_reached && (
                        <div className="flex items-center gap-2 text-xs text-muted-foreground">
                          <Clock3 className="h-3 w-3" />
                          <span>Countdown: {countdownText(workflow.votes.finalize_at)}</span>
                        </div>
                      )}

                      <div className="flex flex-wrap items-center gap-2">
                        {myDecision && <Badge variant="secondary">Your vote: {myDecision}</Badge>}
                        {workflow.votes.decision && <Badge variant="secondary">Decision: {workflow.votes.decision}</Badge>}
                      </div>

                      {pending && (
                        <div className="flex flex-wrap gap-2 pt-1">
                          <Button
                            size="sm"
                            onClick={() => voteWorkflow(workflow.id, "approve")}
                            disabled={Boolean(submittingId)}
                          >
                            {submittingId === workflow.id + "approve" ? "Submitting..." : "Approve"}
                          </Button>
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => voteWorkflow(workflow.id, "deny")}
                            disabled={Boolean(submittingId)}
                          >
                            {submittingId === workflow.id + "deny" ? "Submitting..." : "Deny"}
                          </Button>
                          {user?.isAdmin && (
                            <Button
                              size="sm"
                              variant="secondary"
                              onClick={() => forceApprove(workflow.id)}
                              disabled={Boolean(submittingId)}
                            >
                              {submittingId === workflow.id + "force" ? "Submitting..." : "Force Approve"}
                            </Button>
                          )}
                        </div>
                      )}
                    </CardContent>
                  </Card>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
