"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { ActiveWorkflowListItem, Workflow, WorkflowDeletionProposal } from "@/types/workflow"
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
  const [activeWorkflows, setActiveWorkflows] = useState<ActiveWorkflowListItem[]>([])
  const [deletionProposals, setDeletionProposals] = useState<WorkflowDeletionProposal[]>([])
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
      const [workflowRes, deletionRes, activeRes] = await Promise.all([
        authFetch("/voters/workflows"),
        authFetch("/voters/workflow-deletion-proposals"),
        authFetch("/workflows/active"),
      ])
      if (!workflowRes.ok) throw new Error()

      const workflowData = (await workflowRes.json()) as Workflow[]
      setWorkflows(workflowData || [])

      if (deletionRes.ok) {
        const deletionData = (await deletionRes.json()) as WorkflowDeletionProposal[]
        setDeletionProposals(deletionData || [])
      } else {
        setDeletionProposals([])
      }

      if (activeRes.ok) {
        const activeData = (await activeRes.json()) as ActiveWorkflowListItem[]
        setActiveWorkflows(activeData || [])
      } else {
        setActiveWorkflows([])
      }
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

  const voteDeletionProposal = async (proposalId: string, decision: "approve" | "deny") => {
    setSubmittingId(`deletion:${proposalId}:${decision}`)
    try {
      const res = await authFetch(`/workflow-deletion-proposals/${proposalId}/votes`, {
        method: "POST",
        body: JSON.stringify({ decision }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to submit deletion vote.")
      }
      await loadWorkflows()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to submit deletion vote right now.")
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
  const pendingDeletionCount = useMemo(
    () => deletionProposals.filter((proposal) => proposal.status === "pending").length,
    [deletionProposals],
  )

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
        <p className="text-muted-foreground">Review workflow proposals and deletion proposals, then vote.</p>
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

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Vote className="h-5 w-5" />
            Workflow Deletion Votes
          </CardTitle>
          <CardDescription>
            Pending deletion proposals: <span className="font-medium">{pendingDeletionCount}</span>
          </CardDescription>
        </CardHeader>
        <CardContent>
          {deletionProposals.length === 0 ? (
            <p className="text-sm text-muted-foreground">No workflow deletion proposals available.</p>
          ) : (
            <div className="space-y-4">
              {deletionProposals.map((proposal) => {
                const pending = proposal.status === "pending"
                return (
                  <Card key={proposal.id}>
                    <CardContent className="p-4 space-y-3">
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <div>
                          <h3 className="font-semibold">
                            {proposal.target_type === "series" ? "Series Deletion" : "Workflow Deletion"}
                          </h3>
                          <p className="text-xs text-muted-foreground">
                            Proposal ID: {proposal.id}
                          </p>
                        </div>
                        <Badge variant={pending ? "outline" : proposal.status === "approved" ? "default" : "secondary"}>
                          {proposal.status}
                        </Badge>
                      </div>

                      <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
                        <span>Target Type: {proposal.target_type}</span>
                        <span>Workflow: {proposal.target_workflow_title || proposal.target_workflow_id || "--"}</span>
                        <span>Series: {proposal.target_series_id || "--"}</span>
                      </div>
                      <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
                        <span>
                          Votes: {proposal.votes.approve} approve / {proposal.votes.deny} deny ({proposal.votes.votes_cast}/{proposal.votes.total_voters})
                        </span>
                        <span>Quorum threshold: {proposal.votes.quorum_threshold}</span>
                        <span>Created: {new Date(proposal.created_at).toLocaleString()}</span>
                      </div>

                      {pending && proposal.votes.quorum_reached && (
                        <div className="flex items-center gap-2 text-xs text-muted-foreground">
                          <Clock3 className="h-3 w-3" />
                          <span>Countdown: {countdownText(proposal.votes.finalize_at)}</span>
                        </div>
                      )}

                      <div className="flex flex-wrap items-center gap-2">
                        {proposal.votes.my_decision && <Badge variant="secondary">Your vote: {proposal.votes.my_decision}</Badge>}
                        {proposal.votes.decision && <Badge variant="secondary">Decision: {proposal.votes.decision}</Badge>}
                      </div>

                      {pending && (
                        <div className="flex flex-wrap gap-2 pt-1">
                          <Button
                            size="sm"
                            onClick={() => voteDeletionProposal(proposal.id, "approve")}
                            disabled={Boolean(submittingId)}
                          >
                            {submittingId === `deletion:${proposal.id}:approve` ? "Submitting..." : "Approve"}
                          </Button>
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => voteDeletionProposal(proposal.id, "deny")}
                            disabled={Boolean(submittingId)}
                          >
                            {submittingId === `deletion:${proposal.id}:deny` ? "Submitting..." : "Deny"}
                          </Button>
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

      <Card>
        <CardHeader>
          <CardTitle>Active Workflows</CardTitle>
          <CardDescription>
            Active workflows that can be targeted by proposer deletion proposals.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {activeWorkflows.length === 0 ? (
            <p className="text-sm text-muted-foreground">No active workflows available right now.</p>
          ) : (
            activeWorkflows.map((workflow) => (
              <Card key={`active-${workflow.id}`}>
                <CardContent className="pt-4 space-y-2">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <h4 className="font-semibold">{workflow.title}</h4>
                    <Badge>{workflow.status}</Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">{workflow.description}</p>
                  <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                    <span>Workflow ID: {workflow.id}</span>
                    <span>Series ID: {workflow.series_id}</span>
                    <span>Start: {new Date(workflow.start_at).toLocaleString()}</span>
                  </div>
                </CardContent>
              </Card>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  )
}
