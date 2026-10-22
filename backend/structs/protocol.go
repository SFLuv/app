package structs

// Protocol roles. A role is what an address is *for*, and it is what a
// historical check asks about: "which address was the faucet in March" is a
// question about the role, not about any particular key or Safe.
//
// These strings are also the role values the analytics wallet-role history
// already uses for its env-sourced candidates, so the classification that reads
// that table keeps working when the source becomes the protocol registry.
const (
	ProtocolRoleFaucet        = "faucet"
	ProtocolRoleAdmin         = "admin"
	ProtocolRoleRedeemerAdmin = "redeemer_admin"
	ProtocolRoleZapper        = "zapper"
)

// Protocol account keys. One per function the backend signs for.
const (
	ProtocolAccountFaucet        = "faucet"
	ProtocolAccountRedeemerAdmin = "redeemer_admin"
)

// ProtocolAccount is one of the backend's own smart accounts.
type ProtocolAccount struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Role        string `json:"role"`
	ChainID     int    `json:"chain_id"`
	// SafeAddress is empty until a signer exists to derive it from.
	SafeAddress string `json:"safe_address"`
	OwnerIndex  int    `json:"owner_index"`
	DeployedAt  *int64 `json:"deployed_at"`
	DeployTx    string `json:"deploy_tx"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`

	// Signer is the active signing identity, when there is one.
	Signer *ProtocolSigner `json:"signer"`
}

// ProtocolSigner is one key an account has had. The key material itself never
// appears in this struct: it is read straight out of the sealed column into the
// signer that needs it and never travels any further.
type ProtocolSigner struct {
	ID            int64  `json:"id"`
	AccountKey    string `json:"account_key"`
	EOAAddress    string `json:"eoa_address"`
	CreatedAt     int64  `json:"created_at"`
	ActivatedAt   *int64 `json:"activated_at"`
	ActivationTx  string `json:"activation_tx"`
	RetiredAt     *int64 `json:"retired_at"`
	RetiredReason string `json:"retired_reason"`
	RotatedBy     string `json:"rotated_by"`
}

// ProtocolAddressWindow is one row of "this address held this role between
// these two moments". An open window (EndedAt nil) is the current holder.
type ProtocolAddressWindow struct {
	ID         int64  `json:"id"`
	Address    string `json:"address"`
	Role       string `json:"role"`
	ChainID    int    `json:"chain_id"`
	AccountKey string `json:"account_key"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    *int64 `json:"ended_at"`
	Source     string `json:"source"`
	Note       string `json:"note"`
}

// ProtocolMigrationStep is one entry in the cutover ledger.
type ProtocolMigrationStep struct {
	Step        string `json:"step"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
	TxHash      string `json:"tx_hash"`
	AmountBase  string `json:"amount_base"`
	Attempts    int    `json:"attempts"`
	StartedAt   int64  `json:"started_at"`
	UpdatedAt   int64  `json:"updated_at"`
	CompletedAt *int64 `json:"completed_at"`
}

// Cutover step statuses.
const (
	ProtocolStepPending = "pending"
	ProtocolStepRunning = "running"
	ProtocolStepDone    = "done"
	ProtocolStepFailed  = "failed"
	ProtocolStepSkipped = "skipped"
)

// Where an address-history row came from, for the panel to explain itself.
const (
	ProtocolSourceEnv      = "env"
	ProtocolSourceCutover  = "cutover"
	ProtocolSourceRotation = "rotation"
)

// ProtocolPanelResponse backs the admin Protocol tab.
type ProtocolPanelResponse struct {
	ChainID  int                     `json:"chain_id"`
	Accounts []ProtocolAccount       `json:"accounts"`
	History  []ProtocolAddressWindow `json:"history"`
	Steps    []ProtocolMigrationStep `json:"steps"`
	// MasterKeyConfigured says whether the backend can decrypt its own signer
	// keys. False means every rotation and every sponsored operation will fail,
	// and the panel says so rather than offering buttons that cannot work.
	MasterKeyConfigured bool `json:"master_key_configured"`
	// SweepEnabled and DeployEnabled reflect the env gates. Both default to
	// off, so the panel is where an operator turns the cutover on rather than
	// discovering it already happened.
	SweepEnabled  bool `json:"sweep_enabled"`
	DeployEnabled bool `json:"deploy_enabled"`
	// CutoverComplete is true once every account has a deployed Safe with a
	// live signer, which is when there is nothing on chain left to do.
	CutoverComplete bool `json:"cutover_complete"`
	CanRotate       bool `json:"can_rotate"`
}

// ProtocolRotateResponse is the result of one rotation.
type ProtocolRotateResponse struct {
	AccountKey    string `json:"account_key"`
	SafeAddress   string `json:"safe_address"`
	PreviousEOA   string `json:"previous_eoa"`
	NewEOA        string `json:"new_eoa"`
	OperationHash string `json:"operation_hash"`
	Message       string `json:"message"`
}

// ProtocolRotateAllResponse reports each account separately, because a rotation
// that partly succeeded is the case that matters: the accounts that rotated are
// on new keys and the ones that did not are still on the old ones, and an
// operator needs to know which is which.
type ProtocolRotateAllResponse struct {
	Rotated []ProtocolRotateResponse `json:"rotated"`
	Failed  []ProtocolRotateFailure  `json:"failed"`
}

type ProtocolRotateFailure struct {
	AccountKey string `json:"account_key"`
	Error      string `json:"error"`
}
