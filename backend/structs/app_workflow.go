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
	UserId    string    `json:"user_id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

type WorkflowCreateRequest struct {
	SeriesId    *string                     `json:"series_id,omitempty"`
	Title       string                      `json:"title"`
	Description string                      `json:"description"`
	Recurrence  string                      `json:"recurrence"`
	StartAt     string                      `json:"start_at"`
	Manager     *WorkflowManagerCreateInput `json:"manager,omitempty"`
	Roles       []WorkflowRoleCreateInput   `json:"roles"`
	Steps       []WorkflowStepCreateInput   `json:"steps"`
}

type WorkflowTemplateCreateRequest struct {
	TemplateTitle       string                      `json:"template_title"`
	TemplateDescription string                      `json:"template_description"`
	SeriesId            *string                     `json:"series_id,omitempty"`
	Recurrence          string                      `json:"recurrence"`
	StartAt             string                      `json:"start_at"`
	Manager             *WorkflowManagerCreateInput `json:"manager,omitempty"`
	Roles               []WorkflowRoleCreateInput   `json:"roles"`
	Steps               []WorkflowStepCreateInput   `json:"steps"`
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
	Title        string                        `json:"title"`
	Description  string                        `json:"description"`
	Bounty       uint64                        `json:"bounty"`
	RoleClientId string                        `json:"role_client_id"`
	WorkItems    []WorkflowWorkItemCreateInput `json:"work_items"`
}

type WorkflowWorkItemCreateInput struct {
	Title             string                              `json:"title"`
	Description       string                              `json:"description"`
	Optional          bool                                `json:"optional"`
	RequiresPhoto     bool                                `json:"requires_photo"`
	CameraCaptureOnly bool                                `json:"camera_capture_only"`
	RequiresWritten   bool                                `json:"requires_written_response"`
	RequiresDropdown  bool                                `json:"requires_dropdown"`
	DropdownOptions   []WorkflowDropdownOptionCreateInput `json:"dropdown_options"`
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
	Id                      string         `json:"id"`
	SeriesId                string         `json:"series_id"`
	ProposerId              string         `json:"proposer_id"`
	Title                   string         `json:"title"`
	Description             string         `json:"description"`
	Recurrence              string         `json:"recurrence"`
	StartAt                 time.Time      `json:"start_at"`
	Status                  string         `json:"status"`
	IsStartBlocked          bool           `json:"is_start_blocked"`
	BlockedByWorkflowId     *string        `json:"blocked_by_workflow_id,omitempty"`
	TotalBounty             uint64         `json:"total_bounty"`
	WeeklyBountyRequirement uint64         `json:"weekly_bounty_requirement"`
	BudgetWeeklyDeducted    uint64         `json:"budget_weekly_deducted"`
	BudgetOneTimeDeducted   uint64         `json:"budget_one_time_deducted"`
	VoteQuorumReachedAt     *time.Time     `json:"vote_quorum_reached_at,omitempty"`
	VoteFinalizeAt          *time.Time     `json:"vote_finalize_at,omitempty"`
	VoteFinalizedAt         *time.Time     `json:"vote_finalized_at,omitempty"`
	VoteFinalizedByUserId   *string        `json:"vote_finalized_by_user_id,omitempty"`
	VoteDecision            *string        `json:"vote_decision,omitempty"`
	ManagerRequired         bool           `json:"manager_required"`
	ManagerRoleId           *string        `json:"manager_role_id,omitempty"`
	ManagerImproverId       *string        `json:"manager_improver_id,omitempty"`
	ManagerBounty           uint64         `json:"manager_bounty"`
	ManagerPaidOutAt        *time.Time     `json:"manager_paid_out_at,omitempty"`
	ManagerPayoutError      *string        `json:"manager_payout_error,omitempty"`
	ManagerPayoutLastTryAt  *time.Time     `json:"manager_payout_last_try_at,omitempty"`
	ManagerRetryRequestedAt *time.Time     `json:"manager_retry_requested_at,omitempty"`
	ManagerRetryRequestedBy *string        `json:"manager_retry_requested_by,omitempty"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
	Roles                   []WorkflowRole `json:"roles"`
	Steps                   []WorkflowStep `json:"steps"`
	Votes                   WorkflowVotes  `json:"votes"`
}

type ActiveWorkflowListItem struct {
	Id                      string     `json:"id"`
	SeriesId                string     `json:"series_id"`
	ProposerId              string     `json:"proposer_id"`
	Title                   string     `json:"title"`
	Description             string     `json:"description"`
	Recurrence              string     `json:"recurrence"`
	StartAt                 time.Time  `json:"start_at"`
	Status                  string     `json:"status"`
	IsStartBlocked          bool       `json:"is_start_blocked"`
	BlockedByWorkflowId     *string    `json:"blocked_by_workflow_id,omitempty"`
	TotalBounty             uint64     `json:"total_bounty"`
	WeeklyBountyRequirement uint64     `json:"weekly_bounty_requirement"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	VoteDecision            *string    `json:"vote_decision,omitempty"`
	ApprovedAt              *time.Time `json:"approved_at,omitempty"`
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
	VoteQuorumReachedAt *time.Time    `json:"vote_quorum_reached_at,omitempty"`
	VoteFinalizeAt      *time.Time    `json:"vote_finalize_at,omitempty"`
	VoteFinalizedAt     *time.Time    `json:"vote_finalized_at,omitempty"`
	VoteFinalizedBy     *string       `json:"vote_finalized_by_user_id,omitempty"`
	VoteDecision        *string       `json:"vote_decision,omitempty"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
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
	Id                  string                      `json:"id"`
	TemplateTitle       string                      `json:"template_title"`
	TemplateDescription string                      `json:"template_description"`
	OwnerUserId         *string                     `json:"owner_user_id,omitempty"`
	CreatedByUserId     string                      `json:"created_by_user_id"`
	IsDefault           bool                        `json:"is_default"`
	Recurrence          string                      `json:"recurrence"`
	StartAt             time.Time                   `json:"start_at"`
	SeriesId            *string                     `json:"series_id,omitempty"`
	Manager             *WorkflowManagerCreateInput `json:"manager,omitempty"`
	Roles               []WorkflowRoleCreateInput   `json:"roles"`
	Steps               []WorkflowStepCreateInput   `json:"steps"`
	CreatedAt           time.Time                   `json:"created_at"`
	UpdatedAt           time.Time                   `json:"updated_at"`
}

type WorkflowRole struct {
	Id                  string   `json:"id"`
	WorkflowId          string   `json:"workflow_id"`
	Title               string   `json:"title"`
	RequiredCredentials []string `json:"required_credentials"`
	IsManager           bool     `json:"is_manager"`
}

type WorkflowStep struct {
	Id                 string                  `json:"id"`
	WorkflowId         string                  `json:"workflow_id"`
	StepOrder          int                     `json:"step_order"`
	Title              string                  `json:"title"`
	Description        string                  `json:"description"`
	Bounty             uint64                  `json:"bounty"`
	RoleId             *string                 `json:"role_id,omitempty"`
	AssignedImproverId *string                 `json:"assigned_improver_id,omitempty"`
	Status             string                  `json:"status"`
	StartedAt          *time.Time              `json:"started_at,omitempty"`
	CompletedAt        *time.Time              `json:"completed_at,omitempty"`
	PayoutError        *string                 `json:"payout_error,omitempty"`
	PayoutLastTryAt    *time.Time              `json:"payout_last_try_at,omitempty"`
	RetryRequestedAt   *time.Time              `json:"retry_requested_at,omitempty"`
	RetryRequestedBy   *string                 `json:"retry_requested_by,omitempty"`
	Submission         *WorkflowStepSubmission `json:"submission,omitempty"`
	WorkItems          []WorkflowWorkItem      `json:"work_items"`
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
	RequiresWrittenResponse    bool                     `json:"requires_written_response"`
	RequiresDropdown           bool                     `json:"requires_dropdown"`
	DropdownOptions            []WorkflowDropdownOption `json:"dropdown_options"`
	DropdownRequiresWrittenMap map[string]bool          `json:"dropdown_requires_written_response"`
}

type WorkflowVotes struct {
	Approve         int        `json:"approve"`
	Deny            int        `json:"deny"`
	VotesCast       int        `json:"votes_cast"`
	TotalVoters     int        `json:"total_voters"`
	QuorumReached   bool       `json:"quorum_reached"`
	QuorumThreshold int        `json:"quorum_threshold"`
	QuorumReachedAt *time.Time `json:"quorum_reached_at,omitempty"`
	FinalizeAt      *time.Time `json:"finalize_at,omitempty"`
	FinalizedAt     *time.Time `json:"finalized_at,omitempty"`
	Decision        *string    `json:"decision,omitempty"`
	MyDecision      *string    `json:"my_decision,omitempty"`
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
	Id          string    `json:"id"`
	ImproverId  string    `json:"improver_id"`
	SeriesId    string    `json:"series_id"`
	StepOrder   int       `json:"step_order"`
	AbsentFrom  time.Time `json:"absent_from"`
	AbsentUntil time.Time `json:"absent_until"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ImproverAbsencePeriodCreateRequest struct {
	SeriesId    string `json:"series_id"`
	StepOrder   int    `json:"step_order"`
	AbsentFrom  string `json:"absent_from"`
	AbsentUntil string `json:"absent_until"`
}

type ImproverAbsencePeriodCreateResult struct {
	Absence       ImproverAbsencePeriod `json:"absence"`
	ReleasedCount int                   `json:"released_count"`
	SkippedCount  int                   `json:"skipped_count"`
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
	Id           string    `json:"id"`
	WorkflowId   string    `json:"workflow_id"`
	StepId       string    `json:"step_id"`
	ItemId       string    `json:"item_id"`
	SubmissionId string    `json:"submission_id"`
	FileName     string    `json:"file_name"`
	ContentType  string    `json:"content_type"`
	SizeBytes    int       `json:"size_bytes"`
	CreatedAt    time.Time `json:"created_at"`
}

type WorkflowSubmissionPhotoBlob struct {
	WorkflowSubmissionPhoto
	PhotoData []byte `json:"-"`
}

type WorkflowSubmissionPhotoExport struct {
	Photo     WorkflowSubmissionPhotoBlob `json:"photo"`
	StepOrder int                         `json:"step_order"`
	ItemOrder int                         `json:"item_order"`
	ItemTitle string                      `json:"item_title"`
}

type WorkflowStepSubmission struct {
	Id            string                     `json:"id"`
	WorkflowId    string                     `json:"workflow_id"`
	StepId        string                     `json:"step_id"`
	ImproverId    string                     `json:"improver_id"`
	ItemResponses []WorkflowStepItemResponse `json:"item_responses"`
	SubmittedAt   time.Time                  `json:"submitted_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`
}

type WorkflowStepCompleteRequest struct {
	Items []WorkflowStepItemResponse `json:"items"`
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
	WorkflowId              string    `json:"workflow_id"`
	WorkflowTitle           string    `json:"workflow_title"`
	SeriesId                string    `json:"series_id"`
	Recurrence              string    `json:"recurrence"`
	StartAt                 time.Time `json:"start_at"`
	TotalBounty             uint64    `json:"total_bounty"`
	WeeklyBountyRequirement uint64    `json:"weekly_bounty_requirement"`
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

type UserCredential struct {
	Id             int        `json:"id"`
	UserId         string     `json:"user_id"`
	CredentialType string     `json:"credential_type"`
	IssuedBy       *string    `json:"issued_by,omitempty"`
	IssuedAt       time.Time  `json:"issued_at"`
	IsRevoked      bool       `json:"is_revoked"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
}
