import { formatStatusLabel } from "@/lib/status-labels"

type WorkflowStatusLike = {
  status?: string | null
  start_at?: number | null
}

const upcomingEligibleStatuses = new Set(["approved", "blocked"])

export const getWorkflowDisplayStatus = (workflow: WorkflowStatusLike, nowUnix: number = Date.now() / 1000): string => {
  const status = String(workflow.status || "").trim().toLowerCase()
  if (!status) return ""

  const startAt = Number(workflow.start_at || 0)
  if (startAt > 0 && startAt > nowUnix && upcomingEligibleStatuses.has(status)) {
    return "upcoming"
  }

  return status
}

export const formatWorkflowDisplayStatus = (workflow: WorkflowStatusLike, nowUnix?: number): string => {
  return formatStatusLabel(getWorkflowDisplayStatus(workflow, nowUnix))
}

