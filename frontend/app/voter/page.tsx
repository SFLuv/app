"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { WorkflowDetailsModal } from "@/components/workflows/workflow-details-modal"
import { formatStatusLabel } from "@/lib/status-labels"
import { formatWorkflowDisplayStatus } from "@/lib/workflow-status"
import { ActiveWorkflowListItem, Workflow, WorkflowDeletionProposal, WorkflowDeletionTargetType } from "@/types/workflow"
import { Input } from "@/components/ui/input"
import { AlertTriangle, Clock3, Search, Vote } from "lucide-react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"

function countdownText(finalizeAt?: number | null): string {
  if (!finalizeAt) return "Countdown not started"
  const remainingMs = finalizeAt * 1000 - Date.now()
  if (remainingMs <= 0) return "Finalization pending"

  const hours = Math.floor(remainingMs / (1000 * 60 * 60))
  const minutes = Math.floor((remainingMs % (1000 * 60 * 60)) / (1000 * 60))
  return `${hours}h ${minutes}m remaining`
}

type VoterTab = "workflow-votes" | "deletion-votes" | "active-workflows"

const isVoterTab = (value: string | null): value is VoterTab => {
  return value === "workflow-votes" || value === "deletion-votes" || value === "active-workflows"
}

export default function VoterPage() {
  const searchParams = useSearchParams()
  const pathname = usePathname()
  const router = useRouter()
  const tabFromQuery = searchParams.get("tab")
  const workflowSearchFromQuery = searchParams.get("workflow_search") || ""
  const deletionSearchFromQuery = searchParams.get("deletion_search") || ""
  const { authFetch, status, user } = useApp()
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [activeWorkflows, setActiveWorkflows] = useState<ActiveWorkflowListItem[]>([])
  const [deletionProposals, setDeletionProposals] = useState<WorkflowDeletionProposal[]>([])
  const [loading, setLoading] = useState<boolean>(true)
  const [error, setError] = useState<string>("")
  const [submittingId, setSubmittingId] = useState<string>("")
  const [detailWorkflow, setDetailWorkflow] = useState<Workflow | null>(null)
  const [detailOpen, setDetailOpen] = useState<boolean>(false)
  const [detailLoading, setDetailLoading] = useState<boolean>(false)
  const [detailSource, setDetailSource] = useState<"workflow-votes" | "active-workflows">("workflow-votes")
  const [activeTab, setActiveTab] = useState<VoterTab>(isVoterTab(tabFromQuery) ? tabFromQuery : "workflow-votes")
  const [workflowSearch, setWorkflowSearch] = useState<string>(workflowSearchFromQuery)
  const [deletionSearch, setDeletionSearch] = useState<string>(deletionSearchFromQuery)

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
    const nextTab = searchParams.get("tab")
    if (isVoterTab(nextTab)) {
      setActiveTab((prev) => (nextTab === prev ? prev : nextTab))
    }

    const nextWorkflowSearch = searchParams.get("workflow_search") || ""
    setWorkflowSearch((prev) => (nextWorkflowSearch === prev ? prev : nextWorkflowSearch))

    const nextDeletionSearch = searchParams.get("deletion_search") || ""
    setDeletionSearch((prev) => (nextDeletionSearch === prev ? prev : nextDeletionSearch))
  }, [searchParams])

  useEffect(() => {
    const params = new URLSearchParams(searchParams.toString())
    params.set("tab", activeTab)
    if (workflowSearch) params.set("workflow_search", workflowSearch)
    else params.delete("workflow_search")
    if (deletionSearch) params.set("deletion_search", deletionSearch)
    else params.delete("deletion_search")
    const nextQuery = params.toString()
    if (nextQuery !== searchParams.toString()) {
      router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false })
    }
  }, [activeTab, deletionSearch, pathname, router, searchParams, workflowSearch])

  useEffect(() => {
    if (status !== "authenticated") return
    void loadWorkflows()
  }, [activeTab, status, loadWorkflows])

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

  const getDeletionTargetType = useCallback(
    (workflow: Workflow): WorkflowDeletionTargetType => {
      const appearsSeriesWorkflow =
        workflow.recurrence !== "one_time" ||
        activeWorkflows.some((candidate) => candidate.id !== workflow.id && candidate.series_id === workflow.series_id)
      return appearsSeriesWorkflow ? "series" : "workflow"
    },
    [activeWorkflows],
  )

  const hasPendingDeletionProposal = useCallback(
    (workflow: Workflow, targetType: WorkflowDeletionTargetType): boolean =>
      deletionProposals.some(
        (proposal) =>
          proposal.status === "pending" &&
          ((targetType === "series" &&
            proposal.target_type === "series" &&
            proposal.target_series_id === workflow.series_id) ||
            (targetType === "workflow" &&
              proposal.target_type === "workflow" &&
              proposal.target_workflow_id === workflow.id)),
      ),
    [deletionProposals],
  )

  const proposeDeletionFromActiveWorkflow = async (workflow: Workflow) => {
    const targetType = getDeletionTargetType(workflow)
    if (hasPendingDeletionProposal(workflow, targetType)) {
      setError(targetType === "series" ? "A pending deletion vote already exists for this series." : "A pending deletion vote already exists for this workflow.")
      return
    }

    setSubmittingId(`deletion:create:${workflow.id}`)
    try {
      const res = await authFetch("/voters/workflow-deletion-proposals", {
        method: "POST",
        body: JSON.stringify({
          workflow_id: workflow.id,
          target_type: targetType,
        }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to create deletion proposal.")
      }
      await loadWorkflows()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to create deletion proposal.")
    } finally {
      setSubmittingId("")
    }
  }

  const openWorkflowDetails = async (
    workflowId: string,
    workflow?: Workflow,
    source: "workflow-votes" | "active-workflows" = "workflow-votes",
  ) => {
    setError("")
    setDetailSource(source)

    if (workflow) {
      setDetailWorkflow(workflow)
      setDetailLoading(false)
      setDetailOpen(true)
      return
    }

    const existing = workflows.find((item) => item.id === workflowId)
    if (existing) {
      setDetailWorkflow(existing)
      setDetailLoading(false)
      setDetailOpen(true)
      return
    }

    setDetailWorkflow(null)
    setDetailLoading(true)
    setDetailOpen(true)

    try {
      const res = await authFetch(`/workflows/${workflowId}`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "Unable to load workflow details.")
      }
      const workflowDetails = (await res.json()) as Workflow
      setDetailWorkflow(workflowDetails)
    } catch (err) {
      setDetailOpen(false)
      setError(err instanceof Error ? err.message : "Unable to load workflow details.")
    } finally {
      setDetailLoading(false)
    }
  }

  const pendingCount = useMemo(() => workflows.filter((workflow) => workflow.status === "pending").length, [workflows])
  const pendingDeletionCount = useMemo(
    () => deletionProposals.filter((proposal) => proposal.status === "pending").length,
    [deletionProposals],
  )

  const workflowVotesList = useMemo(
    () => workflows.filter((workflow) => workflow.status !== "approved"),
    [workflows],
  )

  const filteredWorkflows = useMemo(() => {
    const s = workflowSearch.trim().toLowerCase()
    if (!s) return workflowVotesList
    return workflowVotesList.filter((w) => w.title.toLowerCase().includes(s))
  }, [workflowVotesList, workflowSearch])

  const filteredDeletionProposals = useMemo(() => {
    const s = deletionSearch.trim().toLowerCase()
    if (!s) return deletionProposals
    return deletionProposals.filter((p) =>
      (p.target_workflow_title || "").toLowerCase().includes(s) ||
      p.target_type.toLowerCase().includes(s)
    )
  }, [deletionProposals, deletionSearch])

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

      <Tabs value={activeTab} onValueChange={(value) => setActiveTab(value as VoterTab)} className="space-y-4">
        <TabsList className="grid h-auto w-full grid-cols-1 gap-2 p-1 sm:grid-cols-3">
          <TabsTrigger value="workflow-votes">Workflow Votes ({pendingCount})</TabsTrigger>
          <TabsTrigger value="deletion-votes">Deletion Votes ({pendingDeletionCount})</TabsTrigger>
          <TabsTrigger value="active-workflows">Active Workflows</TabsTrigger>
        </TabsList>

        <TabsContent value="workflow-votes">
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
            <CardContent className="space-y-4">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search proposals..."
                  value={workflowSearch}
                  onChange={(e) => setWorkflowSearch(e.target.value)}
                  className="pl-9"
                />
              </div>
              {filteredWorkflows.length === 0 ? (
                <p className="text-sm text-muted-foreground">No workflows available for voting.</p>
              ) : (
                <div className="space-y-4">
                  {filteredWorkflows.map((workflow) => {
                    const pending = workflow.status === "pending"
                    const myDecision = workflow.votes.my_decision

                    return (
                      <Card
                        key={workflow.id}
                        className="cursor-pointer transition-colors hover:bg-muted/30"
                        onClick={() => openWorkflowDetails(workflow.id, workflow, "workflow-votes")}
                      >
                        <CardContent className="p-4 space-y-3">
                          <div className="flex flex-wrap items-center justify-between gap-3">
                            <div>
                              <h3 className="font-semibold">{workflow.title}</h3>
                            </div>
                            <Badge variant={pending ? "outline" : workflow.status === "approved" || workflow.status === "blocked" ? "default" : "secondary"}>
                              {formatWorkflowDisplayStatus(workflow)}
                            </Badge>
                          </div>

                          <p className="text-sm text-muted-foreground">{workflow.description}</p>

                          <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
                            <span>
                              Votes: {workflow.votes.approve} approve / {workflow.votes.deny} deny ({workflow.votes.votes_cast}/{workflow.votes.total_voters})
                            </span>
                            <span>Quorum threshold: {workflow.votes.quorum_threshold}</span>
                            <span>Start: {new Date(workflow.start_at * 1000).toLocaleString()}</span>
                            {workflow.supervisor_required && (
                              <span>Supervisor: {workflow.supervisor_title || workflow.supervisor_organization || "Assigned"}</span>
                            )}
                          </div>

                          {pending && workflow.votes.quorum_reached && (
                            <div className="flex items-center gap-2 text-xs text-muted-foreground">
                              <Clock3 className="h-3 w-3" />
                              <span>Countdown: {countdownText(workflow.votes.finalize_at)}</span>
                            </div>
                          )}

                          <div className="flex flex-wrap items-center gap-2">
                            {myDecision && <Badge variant="secondary">Your vote: {formatStatusLabel(myDecision)}</Badge>}
                            {workflow.votes.decision && <Badge variant="secondary">Decision: {formatStatusLabel(workflow.votes.decision)}</Badge>}
                          </div>

                          {pending && (
                            <div className="flex flex-wrap gap-2 pt-1">
                              <Button
                                className="w-full sm:w-auto"
                                size="sm"
                                onClick={(e) => {
                                  e.stopPropagation()
                                  void voteWorkflow(workflow.id, "approve")
                                }}
                                disabled={Boolean(submittingId)}
                              >
                                {submittingId === workflow.id + "approve" ? "Submitting..." : "Approve"}
                              </Button>
                              <Button
                                className="w-full sm:w-auto"
                                size="sm"
                                variant="outline"
                                onClick={(e) => {
                                  e.stopPropagation()
                                  void voteWorkflow(workflow.id, "deny")
                                }}
                                disabled={Boolean(submittingId)}
                              >
                                {submittingId === workflow.id + "deny" ? "Submitting..." : "Deny"}
                              </Button>
                              {user?.isAdmin && (
                                <Button
                                  className="w-full sm:w-auto"
                                  size="sm"
                                  variant="secondary"
                                  onClick={(e) => {
                                    e.stopPropagation()
                                    void forceApprove(workflow.id)
                                  }}
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
        </TabsContent>

        <TabsContent value="deletion-votes">
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
            <CardContent className="space-y-4">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search deletion proposals..."
                  value={deletionSearch}
                  onChange={(e) => setDeletionSearch(e.target.value)}
                  className="pl-9"
                />
              </div>
              {filteredDeletionProposals.length === 0 ? (
                <p className="text-sm text-muted-foreground">No workflow deletion proposals available.</p>
              ) : (
                <div className="space-y-4">
                  {filteredDeletionProposals.map((proposal) => {
                    const pending = proposal.status === "pending"
                    return (
                      <Card key={proposal.id}>
                        <CardContent className="p-4 space-y-3">
                          <div className="flex flex-wrap items-center justify-between gap-3">
                            <div>
                              <h3 className="font-semibold">
                                {proposal.target_type === "series" ? "Series Deletion" : "Workflow Deletion"}
                              </h3>
                            </div>
                            <Badge variant={pending ? "outline" : proposal.status === "approved" ? "default" : "secondary"}>
                              {formatStatusLabel(proposal.status)}
                            </Badge>
                          </div>

                          <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
                            <span>Target Type: {proposal.target_type}</span>
                            <span>Workflow: {proposal.target_workflow_title || "--"}</span>
                            <span>{proposal.target_type === "series" ? "Target: Entire Series" : "Target: Single Workflow"}</span>
                          </div>
                          <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3">
                            <span>
                              Votes: {proposal.votes.approve} approve / {proposal.votes.deny} deny ({proposal.votes.votes_cast}/{proposal.votes.total_voters})
                            </span>
                            <span>Quorum threshold: {proposal.votes.quorum_threshold}</span>
                            <span>Created: {new Date(proposal.created_at * 1000).toLocaleString()}</span>
                          </div>

                          {pending && proposal.votes.quorum_reached && (
                            <div className="flex items-center gap-2 text-xs text-muted-foreground">
                              <Clock3 className="h-3 w-3" />
                              <span>Countdown: {countdownText(proposal.votes.finalize_at)}</span>
                            </div>
                          )}

                          <div className="flex flex-wrap items-center gap-2">
                            {proposal.votes.my_decision && <Badge variant="secondary">Your vote: {formatStatusLabel(proposal.votes.my_decision)}</Badge>}
                            {proposal.votes.decision && <Badge variant="secondary">Decision: {formatStatusLabel(proposal.votes.decision)}</Badge>}
                          </div>

                          {pending && (
                            <div className="flex flex-wrap gap-2 pt-1">
                              <Button
                                className="w-full sm:w-auto"
                                size="sm"
                                onClick={() => voteDeletionProposal(proposal.id, "approve")}
                                disabled={Boolean(submittingId)}
                              >
                                {submittingId === `deletion:${proposal.id}:approve` ? "Submitting..." : "Approve"}
                              </Button>
                              <Button
                                className="w-full sm:w-auto"
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
        </TabsContent>

        <TabsContent value="active-workflows">
          <Card>
            <CardHeader>
              <CardTitle>Active Workflows</CardTitle>
              <CardDescription>
                Active workflows that can be targeted by voter deletion proposals.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {activeWorkflows.length === 0 ? (
                <p className="text-sm text-muted-foreground">No active workflows available right now.</p>
              ) : (
                activeWorkflows.map((workflow) => (
                  <Card
                    key={`active-${workflow.id}`}
                    className="cursor-pointer transition-colors hover:bg-muted/30"
                    onClick={() => openWorkflowDetails(workflow.id, undefined, "active-workflows")}
                  >
                    <CardContent className="pt-4 space-y-2">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <h4 className="font-semibold">{workflow.title}</h4>
                        <Badge>{formatWorkflowDisplayStatus(workflow)}</Badge>
                      </div>
                      <p className="text-sm text-muted-foreground">{workflow.description}</p>
                      <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                        <span>Start: {new Date(workflow.start_at * 1000).toLocaleString()}</span>
                      </div>
                    </CardContent>
                  </Card>
                ))
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <WorkflowDetailsModal
        workflow={detailWorkflow}
        open={detailOpen}
        onOpenChange={(open) => setDetailOpen(open)}
        loading={detailLoading}
        renderWorkflowActions={
          detailSource === "active-workflows"
            ? (workflow) => {
                const targetType = getDeletionTargetType(workflow)
                const pending = hasPendingDeletionProposal(workflow, targetType)
                const createSubmitting = submittingId === `deletion:create:${workflow.id}`
                return (
                  <div className="space-y-2 rounded-md border bg-secondary/30 p-3">
                    <p className="text-xs text-muted-foreground">
                      {targetType === "series"
                        ? "Deletion proposal will target the full workflow series."
                        : "Deletion proposal will target only this workflow."}
                    </p>
                    <Button
                      className="w-full sm:w-auto"
                      size="sm"
                      onClick={() => void proposeDeletionFromActiveWorkflow(workflow)}
                      disabled={Boolean(submittingId) || pending}
                    >
                      {createSubmitting ? "Submitting..." : pending ? "Deletion Vote Pending" : "Propose Deletion Vote"}
                    </Button>
                  </div>
                )
              }
            : undefined
        }
      />
    </div>
  )
}
