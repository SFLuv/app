export type WorkflowRecurrence = "one_time" | "daily" | "weekly" | "monthly"
export type WorkflowPhotoAspectRatio = "vertical" | "square" | "horizontal"

export type CredentialType = string
export type CredentialVisibility = "public" | "private" | "unlisted"

export interface GlobalCredentialType {
  value: string
  label: string
  visibility?: CredentialVisibility
  badge_content_type?: string | null
  badge_data_base64?: string | null
  created_at: string
  updated_at?: string
}

export interface WorkflowDropdownOption {
  value: string
  label: string
  requires_written_response: boolean
  notify_emails?: string[]
  notify_email_count?: number
  send_pictures_with_email?: boolean
}

export interface WorkflowDropdownOptionCreateInput {
  label: string
  requires_written_response: boolean
  notify_emails: string[]
  notify_email_count?: number
  send_pictures_with_email?: boolean
}

export interface WorkflowWorkItem {
  id: string
  step_id: string
  item_order: number
  title: string
  description: string
  optional: boolean
  requires_photo: boolean
  camera_capture_only: boolean
  photo_required_count: number
  photo_allow_any_count: boolean
  photo_aspect_ratio: WorkflowPhotoAspectRatio
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
  allow_step_not_possible: boolean
  role_id?: string | null
  assigned_improver_id?: string | null
  assigned_improver_name?: string | null
  status: "locked" | "available" | "in_progress" | "completed" | "paid_out"
  started_at?: number | null
  completed_at?: number | null
  payout_error?: string | null
  payout_last_try_at?: number | null
  retry_requested_at?: number | null
  retry_requested_by?: string | null
  submission?: WorkflowStepSubmission | null
  work_items: WorkflowWorkItem[]
}

export interface WorkflowStepSubmission {
  id: string
  workflow_id: string
  step_id: string
  improver_id: string
  step_not_possible: boolean
  step_not_possible_details?: string | null
  item_responses: WorkflowStepItemResponseInput[]
  submitted_at: number
  updated_at: number
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
  quorum_reached_at?: number | null
  finalize_at?: number | null
  finalized_at?: number | null
  decision?: "approve" | "deny" | "admin_approve" | null
  my_decision?: "approve" | "deny" | null
}

export interface Workflow {
  id: string
  series_id: string
  workflow_state_id?: string | null
  proposer_id: string
  title: string
  description: string
  recurrence: WorkflowRecurrence
  recurrence_end_at?: number | null
  start_at: number
  status: "pending" | "approved" | "rejected" | "in_progress" | "completed" | "paid_out" | "blocked" | "expired" | "failed" | "skipped" | "deleted"
  is_start_blocked: boolean
  blocked_by_workflow_id?: string | null
  total_bounty: number
  weekly_bounty_requirement: number
  budget_weekly_deducted: number
  budget_one_time_deducted: number
  vote_quorum_reached_at?: number | null
  vote_finalize_at?: number | null
  vote_finalized_at?: number | null
  vote_finalized_by_user_id?: string | null
  vote_decision?: "approve" | "deny" | "admin_approve" | null
  supervisor_required: boolean
  supervisor_user_id?: string | null
  supervisor_bounty: number
  supervisor_data_fields?: WorkflowSupervisorDataField[]
  supervisor_paid_out_at?: number | null
  supervisor_payout_error?: string | null
  supervisor_payout_last_try_at?: number | null
  supervisor_retry_requested_at?: number | null
  supervisor_retry_requested_by?: string | null
  supervisor_title?: string | null
  supervisor_organization?: string | null
  created_at: number
  updated_at: number
  roles: WorkflowRole[]
  steps: WorkflowStep[]
  votes: WorkflowVotes
}

export interface ActiveWorkflowListItem {
  id: string
  series_id: string
  workflow_state_id?: string | null
  proposer_id: string
  title: string
  description: string
  recurrence: WorkflowRecurrence
  recurrence_end_at?: number | null
  start_at: number
  status: "approved" | "blocked" | "in_progress" | "completed"
  is_start_blocked: boolean
  blocked_by_workflow_id?: string | null
  total_bounty: number
  weekly_bounty_requirement: number
  created_at: number
  updated_at: number
  vote_decision?: "approve" | "deny" | "admin_approve" | null
  approved_at?: number | null
}

export interface AdminWorkflowListItem {
  id: string
  series_id: string
  title: string
  description: string
  recurrence: WorkflowRecurrence
  status: "approved" | "blocked" | "in_progress" | "completed" | "paid_out" | "failed" | "skipped" | "deleted"
  start_at: number
  created_at: number
  updated_at: number
  assigned_improver_emails: string[]
}

export interface AdminWorkflowListResponse {
  items: AdminWorkflowListItem[]
  total: number
  page: number
  count: number
}

export interface WorkflowSeriesClaimant {
  user_id: string
  email: string
  name: string
  claim_count: number
}

export interface WorkflowSeriesClaimRevokeResult {
  series_id: string
  improver_user_id: string
  released_count: number
  skipped_count: number
}

export type WorkflowDeletionTargetType = "workflow" | "series"

export interface WorkflowDeletionProposal {
  id: string
  target_type: WorkflowDeletionTargetType
  target_workflow_id?: string | null
  preview_workflow_id?: string | null
  target_workflow_title?: string | null
  target_series_id?: string | null
  reason: string
  status: "pending" | "approved" | "denied" | "expired"
  requested_by_user_id: string
  vote_quorum_reached_at?: number | null
  vote_finalize_at?: number | null
  vote_finalized_at?: number | null
  vote_finalized_by_user_id?: string | null
  vote_decision?: "approve" | "deny" | "admin_approve" | null
  created_at: number
  updated_at: number
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
  camera_capture_only: boolean
  photo_required_count: number
  photo_allow_any_count: boolean
  photo_aspect_ratio: WorkflowPhotoAspectRatio
  requires_written_response: boolean
  requires_dropdown: boolean
  dropdown_options: WorkflowDropdownOptionCreateInput[]
}

export interface WorkflowStepCreateInput {
  title: string
  description: string
  bounty: number
  role_client_id: string
  allow_step_not_possible: boolean
  work_items: WorkflowWorkItemCreateInput[]
}

export interface WorkflowSupervisorCreateInput {
  user_id: string
  bounty: number
}

export interface WorkflowSupervisorDataField {
  key: string
  value: string
}

export interface WorkflowCreateRequest {
  series_id?: string
  title: string
  description: string
  recurrence: WorkflowRecurrence
  recurrence_end_at?: string
  start_at: string
  supervisor?: WorkflowSupervisorCreateInput
  supervisor_data_fields?: WorkflowSupervisorDataField[]
  roles: WorkflowRoleCreateInput[]
  steps: WorkflowStepCreateInput[]
}

export interface WorkflowEditProposalCreateRequest {
  title: string
  description: string
  supervisor?: WorkflowSupervisorCreateInput
  supervisor_data_fields?: WorkflowSupervisorDataField[]
  roles: WorkflowRoleCreateInput[]
  steps: WorkflowStepCreateInput[]
  reason?: string
}

export interface WorkflowEditProposalVoteRequest {
  decision: "approve" | "deny"
  comment?: string
}

export interface WorkflowEditProposal {
  id: string
  series_id: string
  target_workflow_id?: string | null
  proposed_state_id: string
  requested_by_user_id: string
  reason: string
  status: "pending" | "approved" | "denied" | "expired"
  vote_quorum_reached_at?: number | null
  vote_finalize_at?: number | null
  vote_finalized_at?: number | null
  vote_finalized_by_user_id?: string | null
  vote_decision?: "approve" | "deny" | "admin_approve" | null
  created_at: number
  updated_at: number
  workflow_title: string
  workflow_description: string
  workflow_start_at: number
  recurrence: WorkflowRecurrence
  recurrence_end_at?: number | null
  supervisor_required: boolean
  supervisor_user_id?: string | null
  supervisor_bounty: number
  total_bounty: number
  weekly_bounty_requirement: number
  roles?: WorkflowRoleCreateInput[]
  steps?: WorkflowStepCreateInput[]
  votes: WorkflowVotes
}

export interface WorkflowTemplateCreateRequest {
  template_title: string
  template_description: string
  series_id?: string
  recurrence: WorkflowRecurrence
  supervisor_user_id?: string
  supervisor_bounty?: number
  supervisor_data_fields?: WorkflowSupervisorDataField[]
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
  start_at: number
  series_id?: string | null
  supervisor_user_id?: string | null
  supervisor_bounty?: number | null
  supervisor_data_fields?: WorkflowSupervisorDataField[]
  roles: WorkflowRoleCreateInput[]
  steps: WorkflowStepCreateInput[]
  created_at: number
  updated_at: number
}

export interface ImproverWorkflowFeed {
  active_credentials: CredentialType[]
  workflows: Workflow[]
}

export interface ImproverAbsencePeriod {
  id: string
  improver_id: string
  series_id: string
  step_order: number
  absent_from: number
  absent_until: number
  created_at: number
  updated_at: number
}

export interface ImproverAbsencePeriodCreateRequest {
  series_id: string
  step_order: number
  absent_from: string
  absent_until: string
}

export interface ImproverAbsencePeriodCreateResult {
  absence: ImproverAbsencePeriod
  released_count: number
  skipped_count: number
}

export interface ImproverAbsencePeriodUpdateRequest {
  absent_from: string
  absent_until: string
}

export interface ImproverAbsencePeriodDeleteResult {
  id: string
}

export interface ImproverWorkflowSeriesUnclaimResult {
  series_id: string
  step_order: number
  released_count: number
  skipped_count: number
}

export interface WorkflowStepItemResponseInput {
  item_id: string
  photo_urls?: string[]
  photo_ids?: string[]
  photo_uploads?: WorkflowPhotoUploadInput[]
  photos?: WorkflowSubmissionPhoto[]
  written_response?: string
  dropdown_value?: string
}

export interface WorkflowPhotoUploadInput {
  file_name: string
  content_type: string
  data_base64: string
}

export interface WorkflowSubmissionPhoto {
  id: string
  workflow_id: string
  step_id: string
  item_id: string
  submission_id: string
  file_name: string
  content_type: string
  size_bytes: number
  created_at: number
}

export interface SupervisorWorkflowListItem {
  id: string
  series_id: string
  title: string
  status: Workflow["status"]
  recurrence: WorkflowRecurrence
  start_at: number
  created_at: number
  completed_at?: number | null
  total_bounty: number
  supervisor_bounty: number
}

export interface SupervisorWorkflowListResponse {
  items: SupervisorWorkflowListItem[]
  total: number
  page: number
  count: number
}

export interface SupervisorWorkflowExportRequest {
  workflow_ids: string[]
  date_field: "created_at" | "completed_at" | "start_at" | ""
  date_from: string
  date_to: string
}

export interface WorkflowStepCompleteRequest {
  step_not_possible?: boolean
  step_not_possible_details?: string
  items: WorkflowStepItemResponseInput[]
}
