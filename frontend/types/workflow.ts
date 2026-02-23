export type WorkflowRecurrence = "one_time" | "daily" | "weekly" | "monthly"

export type CredentialType = "dpw_certified" | "sfluv_verifier"

export interface WorkflowDropdownOption {
  value: string
  label: string
  requires_written_response: boolean
  notify_emails?: string[]
}

export interface WorkflowDropdownOptionCreateInput {
  label: string
  requires_written_response: boolean
  notify_emails: string[]
}

export interface WorkflowWorkItem {
  id: string
  step_id: string
  item_order: number
  title: string
  description: string
  optional: boolean
  requires_photo: boolean
  requires_written_response: boolean
  requires_dropdown: boolean
  dropdown_options: WorkflowDropdownOption[]
  dropdown_requires_written_response: Record<string, boolean>
}

export interface WorkflowStep {
  id: string
  workflow_id: string
  step_order: number
  title: string
  description: string
  bounty: number
  role_id?: string | null
  assigned_improver_id?: string | null
  status: "locked" | "available" | "in_progress" | "completed" | "paid_out"
  started_at?: string | null
  completed_at?: string | null
  submission?: WorkflowStepSubmission | null
  work_items: WorkflowWorkItem[]
}

export interface WorkflowStepSubmission {
  id: string
  workflow_id: string
  step_id: string
  improver_id: string
  item_responses: WorkflowStepItemResponseInput[]
  submitted_at: string
  updated_at: string
}

export interface WorkflowRole {
  id: string
  workflow_id: string
  title: string
  required_credentials: CredentialType[]
}

export interface WorkflowVotes {
  approve: number
  deny: number
  votes_cast: number
  total_voters: number
  quorum_reached: boolean
  quorum_threshold: number
  quorum_reached_at?: string | null
  finalize_at?: string | null
  finalized_at?: string | null
  decision?: "approve" | "deny" | "admin_approve" | null
  my_decision?: "approve" | "deny" | null
}

export interface Workflow {
  id: string
  series_id: string
  proposer_id: string
  title: string
  description: string
  recurrence: WorkflowRecurrence
  start_at: string
  status: "pending" | "approved" | "rejected" | "in_progress" | "completed" | "paid_out" | "blocked" | "expired" | "deleted"
  is_start_blocked: boolean
  blocked_by_workflow_id?: string | null
  total_bounty: number
  weekly_bounty_requirement: number
  budget_weekly_deducted: number
  budget_one_time_deducted: number
  vote_quorum_reached_at?: string | null
  vote_finalize_at?: string | null
  vote_finalized_at?: string | null
  vote_finalized_by_user_id?: string | null
  vote_decision?: "approve" | "deny" | "admin_approve" | null
  created_at: string
  updated_at: string
  roles: WorkflowRole[]
  steps: WorkflowStep[]
  votes: WorkflowVotes
}

export interface ActiveWorkflowListItem {
  id: string
  series_id: string
  proposer_id: string
  title: string
  description: string
  recurrence: WorkflowRecurrence
  start_at: string
  status: "approved" | "blocked" | "in_progress" | "completed"
  is_start_blocked: boolean
  blocked_by_workflow_id?: string | null
  total_bounty: number
  weekly_bounty_requirement: number
  created_at: string
  updated_at: string
  vote_decision?: "approve" | "deny" | "admin_approve" | null
  approved_at?: string | null
}

export type WorkflowDeletionTargetType = "workflow" | "series"

export interface WorkflowDeletionProposal {
  id: string
  target_type: WorkflowDeletionTargetType
  target_workflow_id?: string | null
  target_workflow_title?: string | null
  target_series_id?: string | null
  reason: string
  status: "pending" | "approved" | "denied" | "expired"
  requested_by_user_id: string
  vote_quorum_reached_at?: string | null
  vote_finalize_at?: string | null
  vote_finalized_at?: string | null
  vote_finalized_by_user_id?: string | null
  vote_decision?: "approve" | "deny" | "admin_approve" | null
  created_at: string
  updated_at: string
  votes: WorkflowVotes
}

export interface WorkflowRoleCreateInput {
  client_id: string
  title: string
  required_credentials: CredentialType[]
}

export interface WorkflowWorkItemCreateInput {
  title: string
  description: string
  optional: boolean
  requires_photo: boolean
  requires_written_response: boolean
  requires_dropdown: boolean
  dropdown_options: WorkflowDropdownOptionCreateInput[]
}

export interface WorkflowStepCreateInput {
  title: string
  description: string
  bounty: number
  role_client_id: string
  work_items: WorkflowWorkItemCreateInput[]
}

export interface WorkflowCreateRequest {
  series_id?: string
  title: string
  description: string
  recurrence: WorkflowRecurrence
  start_at: string
  roles: WorkflowRoleCreateInput[]
  steps: WorkflowStepCreateInput[]
}

export interface WorkflowTemplateCreateRequest {
  template_title: string
  template_description: string
  series_id?: string
  recurrence: WorkflowRecurrence
  start_at: string
  roles: WorkflowRoleCreateInput[]
  steps: WorkflowStepCreateInput[]
}

export interface WorkflowTemplate {
  id: string
  template_title: string
  template_description: string
  owner_user_id?: string | null
  created_by_user_id: string
  is_default: boolean
  recurrence: WorkflowRecurrence
  start_at: string
  series_id?: string | null
  roles: WorkflowRoleCreateInput[]
  steps: WorkflowStepCreateInput[]
  created_at: string
  updated_at: string
}

export interface ImproverWorkflowFeed {
  active_credentials: CredentialType[]
  workflows: Workflow[]
}

export interface WorkflowStepItemResponseInput {
  item_id: string
  photo_urls: string[]
  written_response?: string
  dropdown_value?: string
}

export interface WorkflowStepCompleteRequest {
  items: WorkflowStepItemResponseInput[]
}
