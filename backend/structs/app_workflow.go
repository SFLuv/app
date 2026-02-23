package structs

import "time"

type CredentialType string

const (
	CredentialDPWCertified  CredentialType = "dpw_certified"
	CredentialSFLUVVerifier CredentialType = "sfluv_verifier"
)

func IsValidCredentialType(value string) bool {
	switch CredentialType(value) {
	case CredentialDPWCertified, CredentialSFLUVVerifier:
		return true
	default:
		return false
	}
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
	SeriesId    *string                   `json:"series_id,omitempty"`
	Title       string                    `json:"title"`
	Description string                    `json:"description"`
	Recurrence  string                    `json:"recurrence"`
	StartAt     string                    `json:"start_at"`
	Roles       []WorkflowRoleCreateInput `json:"roles"`
	Steps       []WorkflowStepCreateInput `json:"steps"`
}

type WorkflowTemplateCreateRequest struct {
	TemplateTitle       string                    `json:"template_title"`
	TemplateDescription string                    `json:"template_description"`
	SeriesId            *string                   `json:"series_id,omitempty"`
	Recurrence          string                    `json:"recurrence"`
	StartAt             string                    `json:"start_at"`
	Roles               []WorkflowRoleCreateInput `json:"roles"`
	Steps               []WorkflowStepCreateInput `json:"steps"`
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
	Title            string                              `json:"title"`
	Description      string                              `json:"description"`
	Optional         bool                                `json:"optional"`
	RequiresPhoto    bool                                `json:"requires_photo"`
	RequiresWritten  bool                                `json:"requires_written_response"`
	RequiresDropdown bool                                `json:"requires_dropdown"`
	DropdownOptions  []WorkflowDropdownOptionCreateInput `json:"dropdown_options"`
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
	Id                  string                    `json:"id"`
	TemplateTitle       string                    `json:"template_title"`
	TemplateDescription string                    `json:"template_description"`
	OwnerUserId         *string                   `json:"owner_user_id,omitempty"`
	CreatedByUserId     string                    `json:"created_by_user_id"`
	IsDefault           bool                      `json:"is_default"`
	Recurrence          string                    `json:"recurrence"`
	StartAt             time.Time                 `json:"start_at"`
	SeriesId            *string                   `json:"series_id,omitempty"`
	Roles               []WorkflowRoleCreateInput `json:"roles"`
	Steps               []WorkflowStepCreateInput `json:"steps"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

type WorkflowRole struct {
	Id                  string   `json:"id"`
	WorkflowId          string   `json:"workflow_id"`
	Title               string   `json:"title"`
	RequiredCredentials []string `json:"required_credentials"`
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

type WorkflowStepItemResponse struct {
	ItemId          string   `json:"item_id"`
	PhotoURLs       []string `json:"photo_urls"`
	WrittenResponse *string  `json:"written_response,omitempty"`
	DropdownValue   *string  `json:"dropdown_value,omitempty"`
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

type UserCredential struct {
	Id             int        `json:"id"`
	UserId         string     `json:"user_id"`
	CredentialType string     `json:"credential_type"`
	IssuedBy       *string    `json:"issued_by,omitempty"`
	IssuedAt       time.Time  `json:"issued_at"`
	IsRevoked      bool       `json:"is_revoked"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
}
