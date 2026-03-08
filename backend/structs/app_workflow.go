package structs

import "time"

type CredentialType = string

type Issuer struct {
	UserId       string    `json:"user_id"`
	Organization string    `json:"organization"`
	Email        string    `json:"email"`
	Nickname     *string   `json:"nickname"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type IssuerRequest struct {
	Organization string `json:"organization"`
	Email        string `json:"email"`
}

type IssuerUpdateRequest struct {
	UserId   string  `json:"user_id"`
	Status   *string `json:"status,omitempty"`
	Nickname *string `json:"nickname,omitempty"`
}

type GlobalCredentialType struct {
	Value     string    `json:"value"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

type GlobalCredentialTypeRequest struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Proposer struct {
	UserId       string    `json:"user_id"`
	Organization string    `json:"organization"`
	Email        string    `json:"email"`
	Nickname     *string   `json:"nickname"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ProposerRequest struct {
	Organization string `json:"organization"`
	Email        string `json:"email"`
}

type ProposerUpdateRequest struct {
	UserId   string  `json:"user_id"`
	Status   *string `json:"status,omitempty"`
	Nickname *string `json:"nickname,omitempty"`
}

type Improver struct {
	UserId                string    `json:"user_id"`
	FirstName             string    `json:"first_name"`
	LastName              string    `json:"last_name"`
	Email                 string    `json:"email"`
	PrimaryRewardsAccount string    `json:"primary_rewards_account"`
	Status                string    `json:"status"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type ImproverRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

type ImproverUpdateRequest struct {
	UserId string  `json:"user_id"`
	Status *string `json:"status,omitempty"`
}

type PrimaryRewardsAccountUpdateRequest struct {
	PrimaryRewardsAccount string `json:"primary_rewards_account"`
}

type Supervisor struct {
	UserId                string    `json:"user_id"`
	Organization          string    `json:"organization"`
	Email                 string    `json:"email"`
	PrimaryRewardsAccount string    `json:"primary_rewards_account"`
	Nickname              *string   `json:"nickname"`
	Status                string    `json:"status"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type SupervisorRequest struct {
	Organization string `json:"organization"`
	Email        string `json:"email"`
}

type SupervisorUpdateRequest struct {
	UserId   string  `json:"user_id"`
	Status   *string `json:"status,omitempty"`
	Nickname *string `json:"nickname,omitempty"`
}

type WorkflowCreateRequest struct {
	SeriesId             *string                        `json:"series_id,omitempty"`
	Title                string                         `json:"title"`
	Description          string                         `json:"description"`
	Recurrence           string                         `json:"recurrence"`
	StartAt              string                         `json:"start_at"`
	Supervisor           *WorkflowSupervisorCreateInput `json:"supervisor,omitempty"`
	SupervisorDataFields []WorkflowSupervisorDataField  `json:"supervisor_data_fields,omitempty"`
	Manager              *WorkflowManagerCreateInput    `json:"manager,omitempty"`
	Roles                []WorkflowRoleCreateInput      `json:"roles"`
	Steps                []WorkflowStepCreateInput      `json:"steps"`
}

type WorkflowTemplateCreateRequest struct {
	TemplateTitle        string                        `json:"template_title"`
	TemplateDescription  string                        `json:"template_description"`
	SeriesId             *string                       `json:"series_id,omitempty"`
	Recurrence           string                        `json:"recurrence"`
	StartAt              string                        `json:"start_at"`
	SupervisorUserId     *string                       `json:"supervisor_user_id,omitempty"`
	SupervisorBounty     *uint64                       `json:"supervisor_bounty,omitempty"`
	SupervisorDataFields []WorkflowSupervisorDataField `json:"supervisor_data_fields,omitempty"`
	Manager              *WorkflowManagerCreateInput   `json:"manager,omitempty"`
	Roles                []WorkflowRoleCreateInput     `json:"roles"`
	Steps                []WorkflowStepCreateInput     `json:"steps"`
}

type WorkflowSupervisorCreateInput struct {
	UserId string `json:"user_id"`
	Bounty uint64 `json:"bounty"`
}

type WorkflowSupervisorDataField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type WorkflowManagerCreateInput struct {
	RequiredCredentials []string `json:"required_credentials"`
	Bounty              uint64   `json:"bounty"`
}

type WorkflowRoleCreateInput struct {
	ClientId            string   `json:"client_id"`
	Title               string   `json:"title"`
	RequiredCredentials []string `json:"required_credentials"`
}

type WorkflowStepCreateInput struct {
	Title                string                        `json:"title"`
	Description          string                        `json:"description"`
	Bounty               uint64                        `json:"bounty"`
	RoleClientId         string                        `json:"role_client_id"`
	AllowStepNotPossible bool                          `json:"allow_step_not_possible"`
	WorkItems            []WorkflowWorkItemCreateInput `json:"work_items"`
}

type WorkflowWorkItemCreateInput struct {
	Title              string                              `json:"title"`
	Description        string                              `json:"description"`
	Optional           bool                                `json:"optional"`
	RequiresPhoto      bool                                `json:"requires_photo"`
	CameraCaptureOnly  bool                                `json:"camera_capture_only"`
	PhotoRequiredCount int                                 `json:"photo_required_count"`
	PhotoAllowAnyCount bool                                `json:"photo_allow_any_count"`
	PhotoAspectRatio   string                              `json:"photo_aspect_ratio"`
	RequiresWritten    bool                                `json:"requires_written_response"`
	RequiresDropdown   bool                                `json:"requires_dropdown"`
	DropdownOptions    []WorkflowDropdownOptionCreateInput `json:"dropdown_options"`
}

type WorkflowDropdownOptionCreateInput struct {
	Label                   string   `json:"label"`
	RequiresWrittenResponse bool     `json:"requires_written_response"`
	NotifyEmails            []string `json:"notify_emails"`
}

type WorkflowDropdownOption struct {
	Value                   string   `json:"value"`
	Label                   string   `json:"label"`
	RequiresWrittenResponse bool     `json:"requires_written_response"`
	NotifyEmails            []string `json:"notify_emails,omitempty"`
	NotifyEmailCount        int      `json:"notify_email_count,omitempty"`
}

type Workflow struct {
	Id                         string                        `json:"id"`
	SeriesId                   string                        `json:"series_id"`
	ProposerId                 string                        `json:"proposer_id"`
	Title                      string                        `json:"title"`
	Description                string                        `json:"description"`
	Recurrence                 string                        `json:"recurrence"`
	StartAt                    int64                         `json:"start_at"`
	Status                     string                        `json:"status"`
	IsStartBlocked             bool                          `json:"is_start_blocked"`
	BlockedByWorkflowId        *string                       `json:"blocked_by_workflow_id,omitempty"`
	TotalBounty                uint64                        `json:"total_bounty"`
	WeeklyBountyRequirement    uint64                        `json:"weekly_bounty_requirement"`
	BudgetWeeklyDeducted       uint64                        `json:"budget_weekly_deducted"`
	BudgetOneTimeDeducted      uint64                        `json:"budget_one_time_deducted"`
	VoteQuorumReachedAt        *int64                        `json:"vote_quorum_reached_at,omitempty"`
	VoteFinalizeAt             *int64                        `json:"vote_finalize_at,omitempty"`
	VoteFinalizedAt            *int64                        `json:"vote_finalized_at,omitempty"`
	VoteFinalizedByUserId      *string                       `json:"vote_finalized_by_user_id,omitempty"`
	VoteDecision               *string                       `json:"vote_decision,omitempty"`
	SupervisorRequired         bool                          `json:"supervisor_required"`
	SupervisorUserId           *string                       `json:"supervisor_user_id,omitempty"`
	SupervisorBounty           uint64                        `json:"supervisor_bounty"`
	SupervisorDataFields       []WorkflowSupervisorDataField `json:"supervisor_data_fields,omitempty"`
	SupervisorPaidOutAt        *int64                        `json:"supervisor_paid_out_at,omitempty"`
	SupervisorPayoutError      *string                       `json:"supervisor_payout_error,omitempty"`
	SupervisorPayoutLastTryAt  *int64                        `json:"supervisor_payout_last_try_at,omitempty"`
	SupervisorRetryRequestedAt *int64                        `json:"supervisor_retry_requested_at,omitempty"`
	SupervisorRetryRequestedBy *string                       `json:"supervisor_retry_requested_by,omitempty"`
	SupervisorTitle            *string                       `json:"supervisor_title,omitempty"`
	SupervisorOrganization     *string                       `json:"supervisor_organization,omitempty"`
	ManagerRequired            bool                          `json:"-"`
	ManagerRoleId              *string                       `json:"-"`
	ManagerImproverId          *string                       `json:"-"`
	ManagerBounty              uint64                        `json:"-"`
	ManagerPaidOutAt           *int64                        `json:"-"`
	ManagerPayoutError         *string                       `json:"-"`
	ManagerPayoutLastTryAt     *int64                        `json:"-"`
	ManagerPayoutInProgress    bool                          `json:"-"`
	ManagerRetryRequestedAt    *int64                        `json:"-"`
	ManagerRetryRequestedBy    *string                       `json:"-"`
	CreatedAt                  int64                         `json:"created_at"`
	UpdatedAt                  int64                         `json:"updated_at"`
	Roles                      []WorkflowRole                `json:"roles"`
	Steps                      []WorkflowStep                `json:"steps"`
	Votes                      WorkflowVotes                 `json:"votes"`
}

type ActiveWorkflowListItem struct {
	Id                      string  `json:"id"`
	SeriesId                string  `json:"series_id"`
	ProposerId              string  `json:"proposer_id"`
	Title                   string  `json:"title"`
	Description             string  `json:"description"`
	Recurrence              string  `json:"recurrence"`
	StartAt                 int64   `json:"start_at"`
	Status                  string  `json:"status"`
	IsStartBlocked          bool    `json:"is_start_blocked"`
	BlockedByWorkflowId     *string `json:"blocked_by_workflow_id,omitempty"`
	TotalBounty             uint64  `json:"total_bounty"`
	WeeklyBountyRequirement uint64  `json:"weekly_bounty_requirement"`
	CreatedAt               int64   `json:"created_at"`
	UpdatedAt               int64   `json:"updated_at"`
	VoteDecision            *string `json:"vote_decision,omitempty"`
	ApprovedAt              *int64  `json:"approved_at,omitempty"`
}

type AdminWorkflowListItem struct {
	Id                     string   `json:"id"`
	SeriesId               string   `json:"series_id"`
	Title                  string   `json:"title"`
	Description            string   `json:"description"`
	Recurrence             string   `json:"recurrence"`
	Status                 string   `json:"status"`
	StartAt                int64    `json:"start_at"`
	CreatedAt              int64    `json:"created_at"`
	UpdatedAt              int64    `json:"updated_at"`
	AssignedImproverEmails []string `json:"assigned_improver_emails"`
}

type AdminWorkflowListResponse struct {
	Items []*AdminWorkflowListItem `json:"items"`
	Total int                      `json:"total"`
	Page  int                      `json:"page"`
	Count int                      `json:"count"`
}

type WorkflowSeriesClaimant struct {
	UserId     string `json:"user_id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	ClaimCount int    `json:"claim_count"`
}

type WorkflowSeriesClaimRevokeRequest struct {
	ImproverUserId string `json:"improver_user_id"`
}

type WorkflowSeriesClaimRevokeResult struct {
	SeriesId       string `json:"series_id"`
	ImproverUserId string `json:"improver_user_id"`
	ReleasedCount  int    `json:"released_count"`
	SkippedCount   int    `json:"skipped_count"`
}

type WorkflowDeletionProposalCreateRequest struct {
	WorkflowId string `json:"workflow_id"`
	TargetType string `json:"target_type"`
	Reason     string `json:"reason,omitempty"`
}

type WorkflowDeletionProposalVoteRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment,omitempty"`
}

type WorkflowDeletionProposal struct {
	Id                  string        `json:"id"`
	TargetType          string        `json:"target_type"`
	TargetWorkflowId    *string       `json:"target_workflow_id,omitempty"`
	TargetWorkflowTitle *string       `json:"target_workflow_title,omitempty"`
	TargetSeriesId      *string       `json:"target_series_id,omitempty"`
	Reason              string        `json:"reason"`
	Status              string        `json:"status"`
	RequestedByUserId   string        `json:"requested_by_user_id"`
	VoteQuorumReachedAt *int64        `json:"vote_quorum_reached_at,omitempty"`
	VoteFinalizeAt      *int64        `json:"vote_finalize_at,omitempty"`
	VoteFinalizedAt     *int64        `json:"vote_finalized_at,omitempty"`
	VoteFinalizedBy     *string       `json:"vote_finalized_by_user_id,omitempty"`
	VoteDecision        *string       `json:"vote_decision,omitempty"`
	CreatedAt           int64         `json:"created_at"`
	UpdatedAt           int64         `json:"updated_at"`
	Votes               WorkflowVotes `json:"votes"`
}

type WorkflowProposalExpiryNotice struct {
	WorkflowId     string `json:"workflow_id"`
	WorkflowTitle  string `json:"workflow_title"`
	ProposerUserId string `json:"proposer_user_id"`
	ProposerEmail  string `json:"proposer_email"`
}

type WorkflowProposalOutcomeNotification struct {
	WorkflowId     string `json:"workflow_id"`
	WorkflowTitle  string `json:"workflow_title"`
	Decision       string `json:"decision"`
	ProposerUserId string `json:"proposer_user_id"`
	ProposerEmail  string `json:"proposer_email"`
}

type WorkflowTemplate struct {
	Id                   string                        `json:"id"`
	TemplateTitle        string                        `json:"template_title"`
	TemplateDescription  string                        `json:"template_description"`
	OwnerUserId          *string                       `json:"owner_user_id,omitempty"`
	CreatedByUserId      string                        `json:"created_by_user_id"`
	IsDefault            bool                          `json:"is_default"`
	Recurrence           string                        `json:"recurrence"`
	StartAt              int64                         `json:"start_at"`
	SeriesId             *string                       `json:"series_id,omitempty"`
	SupervisorUserId     *string                       `json:"supervisor_user_id,omitempty"`
	SupervisorBounty     *uint64                       `json:"supervisor_bounty,omitempty"`
	SupervisorDataFields []WorkflowSupervisorDataField `json:"supervisor_data_fields,omitempty"`
	Manager              *WorkflowManagerCreateInput   `json:"-"`
	Roles                []WorkflowRoleCreateInput     `json:"roles"`
	Steps                []WorkflowStepCreateInput     `json:"steps"`
	CreatedAt            int64                         `json:"created_at"`
	UpdatedAt            int64                         `json:"updated_at"`
}

type WorkflowRole struct {
	Id                  string   `json:"id"`
	WorkflowId          string   `json:"workflow_id"`
	Title               string   `json:"title"`
	RequiredCredentials []string `json:"required_credentials"`
	IsManager           bool     `json:"-"`
}

type WorkflowStep struct {
	Id                   string                  `json:"id"`
	WorkflowId           string                  `json:"workflow_id"`
	StepOrder            int                     `json:"step_order"`
	Title                string                  `json:"title"`
	Description          string                  `json:"description"`
	Bounty               uint64                  `json:"bounty"`
	AllowStepNotPossible bool                    `json:"allow_step_not_possible"`
	RoleId               *string                 `json:"role_id,omitempty"`
	AssignedImproverId   *string                 `json:"assigned_improver_id,omitempty"`
	AssignedImproverName *string                 `json:"assigned_improver_name,omitempty"`
	Status               string                  `json:"status"`
	StartedAt            *int64                  `json:"started_at,omitempty"`
	CompletedAt          *int64                  `json:"completed_at,omitempty"`
	PayoutError          *string                 `json:"payout_error,omitempty"`
	PayoutLastTryAt      *int64                  `json:"payout_last_try_at,omitempty"`
	PayoutInProgress     bool                    `json:"-"`
	RetryRequestedAt     *int64                  `json:"retry_requested_at,omitempty"`
	RetryRequestedBy     *string                 `json:"retry_requested_by,omitempty"`
	Submission           *WorkflowStepSubmission `json:"submission,omitempty"`
	WorkItems            []WorkflowWorkItem      `json:"work_items"`
}

type WorkflowWorkItem struct {
	Id                         string                   `json:"id"`
	StepId                     string                   `json:"step_id"`
	ItemOrder                  int                      `json:"item_order"`
	Title                      string                   `json:"title"`
	Description                string                   `json:"description"`
	Optional                   bool                     `json:"optional"`
	RequiresPhoto              bool                     `json:"requires_photo"`
	CameraCaptureOnly          bool                     `json:"camera_capture_only"`
	PhotoRequiredCount         int                      `json:"photo_required_count"`
	PhotoAllowAnyCount         bool                     `json:"photo_allow_any_count"`
	PhotoAspectRatio           string                   `json:"photo_aspect_ratio"`
	RequiresWrittenResponse    bool                     `json:"requires_written_response"`
	RequiresDropdown           bool                     `json:"requires_dropdown"`
	DropdownOptions            []WorkflowDropdownOption `json:"dropdown_options"`
	DropdownRequiresWrittenMap map[string]bool          `json:"dropdown_requires_written_response"`
}

type WorkflowVotes struct {
	Approve         int     `json:"approve"`
	Deny            int     `json:"deny"`
	VotesCast       int     `json:"votes_cast"`
	TotalVoters     int     `json:"total_voters"`
	QuorumReached   bool    `json:"quorum_reached"`
	QuorumThreshold int     `json:"quorum_threshold"`
	QuorumReachedAt *int64  `json:"quorum_reached_at,omitempty"`
	FinalizeAt      *int64  `json:"finalize_at,omitempty"`
	FinalizedAt     *int64  `json:"finalized_at,omitempty"`
	Decision        *string `json:"decision,omitempty"`
	MyDecision      *string `json:"my_decision,omitempty"`
}

type WorkflowVoteRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment,omitempty"`
}

type WorkflowStatusUpdate struct {
	WorkflowId string `json:"workflow_id"`
	Status     string `json:"status"`
}

type ImproverWorkflowFeed struct {
	ActiveCredentials []string    `json:"active_credentials"`
	Workflows         []*Workflow `json:"workflows"`
}

type WorkflowStepClaimRequest struct {
	StepId string `json:"step_id"`
}

type ImproverAbsencePeriod struct {
	Id          string `json:"id"`
	ImproverId  string `json:"improver_id"`
	SeriesId    string `json:"series_id"`
	StepOrder   int    `json:"step_order"`
	AbsentFrom  int64  `json:"absent_from"`
	AbsentUntil int64  `json:"absent_until"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type ImproverAbsencePeriodCreateRequest struct {
	SeriesId    string `json:"series_id"`
	StepOrder   int    `json:"step_order"`
	AbsentFrom  string `json:"absent_from"`
	AbsentUntil string `json:"absent_until"`
}

type ImproverAbsencePeriodUpdateRequest struct {
	AbsentFrom  string `json:"absent_from"`
	AbsentUntil string `json:"absent_until"`
}

type ImproverAbsencePeriodCreateResult struct {
	Absence       ImproverAbsencePeriod `json:"absence"`
	ReleasedCount int                   `json:"released_count"`
	SkippedCount  int                   `json:"skipped_count"`
}

type ImproverAbsencePeriodDeleteResult struct {
	Id string `json:"id"`
}

type ImproverWorkflowSeriesUnclaimRequest struct {
	SeriesId  string `json:"series_id"`
	StepOrder int    `json:"step_order"`
}

type ImproverWorkflowSeriesUnclaimResult struct {
	SeriesId      string `json:"series_id"`
	StepOrder     int    `json:"step_order"`
	ReleasedCount int    `json:"released_count"`
	SkippedCount  int    `json:"skipped_count"`
}

type WorkflowStepItemResponse struct {
	ItemId          string                    `json:"item_id"`
	PhotoURLs       []string                  `json:"photo_urls,omitempty"` // legacy compatibility for older submissions.
	PhotoIDs        []string                  `json:"photo_ids,omitempty"`
	PhotoUploads    []WorkflowPhotoUpload     `json:"photo_uploads,omitempty"`
	Photos          []WorkflowSubmissionPhoto `json:"photos,omitempty"`
	WrittenResponse *string                   `json:"written_response,omitempty"`
	DropdownValue   *string                   `json:"dropdown_value,omitempty"`
}

type WorkflowPhotoUpload struct {
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	DataBase64  string `json:"data_base64"`
}

type WorkflowSubmissionPhoto struct {
	Id           string `json:"id"`
	WorkflowId   string `json:"workflow_id"`
	StepId       string `json:"step_id"`
	ItemId       string `json:"item_id"`
	SubmissionId string `json:"submission_id"`
	FileName     string `json:"file_name"`
	ContentType  string `json:"content_type"`
	SizeBytes    int    `json:"size_bytes"`
	CreatedAt    int64  `json:"created_at"`
}

type WorkflowSubmissionPhotoBlob struct {
	WorkflowSubmissionPhoto
	PhotoData []byte `json:"-"`
}

type WorkflowSubmissionPhotoExport struct {
	Photo           WorkflowSubmissionPhotoBlob `json:"photo"`
	StepOrder       int                         `json:"step_order"`
	ItemOrder       int                         `json:"item_order"`
	ItemTitle       string                      `json:"item_title"`
	WorkflowTitle   string                      `json:"workflow_title"`
	WorkflowStartAt int64                       `json:"workflow_start_at"`
}

type WorkflowStepSubmission struct {
	Id                     string                     `json:"id"`
	WorkflowId             string                     `json:"workflow_id"`
	StepId                 string                     `json:"step_id"`
	ImproverId             string                     `json:"improver_id"`
	StepNotPossible        bool                       `json:"step_not_possible"`
	StepNotPossibleDetails *string                    `json:"step_not_possible_details,omitempty"`
	ItemResponses          []WorkflowStepItemResponse `json:"item_responses"`
	SubmittedAt            int64                      `json:"submitted_at"`
	UpdatedAt              int64                      `json:"updated_at"`
}

type WorkflowStepCompleteRequest struct {
	StepNotPossible        bool                       `json:"step_not_possible"`
	StepNotPossibleDetails *string                    `json:"step_not_possible_details,omitempty"`
	Items                  []WorkflowStepItemResponse `json:"items"`
}

type AdminWorkflowPayoutResolutionRequest struct {
	TargetType   string `json:"target_type"`
	Action       string `json:"action"`
	StepId       string `json:"step_id,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type WorkflowStepAvailabilityNotification struct {
	WorkflowId    string `json:"workflow_id"`
	WorkflowTitle string `json:"workflow_title"`
	StepId        string `json:"step_id"`
	StepTitle     string `json:"step_title"`
	UserId        string `json:"user_id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
}

type WorkflowSeriesStartFundingCheck struct {
	WorkflowId              string `json:"workflow_id"`
	WorkflowTitle           string `json:"workflow_title"`
	SeriesId                string `json:"series_id"`
	Recurrence              string `json:"recurrence"`
	StartAt                 int64  `json:"start_at"`
	TotalBounty             uint64 `json:"total_bounty"`
	WeeklyBountyRequirement uint64 `json:"weekly_bounty_requirement"`
}

type WorkflowStartRefreshResult struct {
	AvailabilityNotifications []WorkflowStepAvailabilityNotification `json:"availability_notifications"`
	SeriesFundingChecks       []WorkflowSeriesStartFundingCheck      `json:"series_funding_checks"`
}

type WorkflowDropdownNotification struct {
	WorkflowId    string   `json:"workflow_id"`
	WorkflowTitle string   `json:"workflow_title"`
	StepId        string   `json:"step_id"`
	StepTitle     string   `json:"step_title"`
	ItemId        string   `json:"item_id"`
	ItemTitle     string   `json:"item_title"`
	DropdownValue string   `json:"dropdown_value"`
	Emails        []string `json:"emails"`
}

type WorkflowStepCompletionResult struct {
	WorkflowStatus            string                                 `json:"workflow_status"`
	AvailabilityNotifications []WorkflowStepAvailabilityNotification `json:"availability_notifications"`
	DropdownNotifications     []WorkflowDropdownNotification         `json:"dropdown_notifications"`
}

type IssuerWithScopes struct {
	UserId             string   `json:"user_id"`
	IsIssuer           bool     `json:"is_issuer"`
	AllowedCredentials []string `json:"allowed_credentials"`
	Organization       string   `json:"organization"`
	Nickname           *string  `json:"nickname"`
}

type IssuerScopeUpdateRequest struct {
	UserId             string   `json:"user_id"`
	AllowedCredentials []string `json:"allowed_credentials"`
	MakeIssuer         *bool    `json:"make_issuer,omitempty"`
}

type CredentialIssueRequest struct {
	UserId         string `json:"user_id"`
	CredentialType string `json:"credential_type"`
}

type CredentialRequest struct {
	Id                 string     `json:"id"`
	UserId             string     `json:"user_id"`
	CredentialType     string     `json:"credential_type"`
	Status             string     `json:"status"`
	RequestedAt        time.Time  `json:"requested_at"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy         *string    `json:"resolved_by,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	RequesterName      string     `json:"requester_name"`
	RequesterFirstName string     `json:"requester_first_name"`
	RequesterLastName  string     `json:"requester_last_name"`
	RequesterEmail     string     `json:"requester_email"`
}

type CredentialRequestCreateRequest struct {
	CredentialType string `json:"credential_type"`
}

type CredentialRequestDecisionRequest struct {
	Decision string `json:"decision"`
	Status   string `json:"status,omitempty"`
}

type CredentialRequestIssuerRecipient struct {
	UserId string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}

type SupervisorWorkflowListItem struct {
	Id               string `json:"id"`
	SeriesId         string `json:"series_id"`
	Title            string `json:"title"`
	Status           string `json:"status"`
	Recurrence       string `json:"recurrence"`
	StartAt          int64  `json:"start_at"`
	CreatedAt        int64  `json:"created_at"`
	CompletedAt      *int64 `json:"completed_at,omitempty"`
	TotalBounty      uint64 `json:"total_bounty"`
	SupervisorBounty uint64 `json:"supervisor_bounty"`
}

type SupervisorWorkflowListResponse struct {
	Items []*SupervisorWorkflowListItem `json:"items"`
	Total int                           `json:"total"`
	Page  int                           `json:"page"`
	Count int                           `json:"count"`
}

type SupervisorWorkflowExportRequest struct {
	WorkflowIds []string `json:"workflow_ids"`
	DateField   string   `json:"date_field"`
	DateFrom    string   `json:"date_from"`
	DateTo      string   `json:"date_to"`
}

type UserCredential struct {
	Id             int        `json:"id"`
	UserId         string     `json:"user_id"`
	CredentialType string     `json:"credential_type"`
	IssuedBy       *string    `json:"issued_by,omitempty"`
	IssuedAt       time.Time  `json:"issued_at"`
	IsRevoked      bool       `json:"is_revoked"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
}
