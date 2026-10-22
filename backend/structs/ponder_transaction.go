package structs

type PonderTransaction struct {
	Id        string `json:"id"`
	ChainID   int64  `json:"chain_id"`
	Hash      string `json:"hash"`
	Amount    string `json:"amount"`
	Timestamp uint64 `json:"timestamp"`
	From      string `json:"from"`
	To        string `json:"to"`
	Memo      string `json:"memo,omitempty"`
	// Type is what the backend knows the transfer was: "volunteer_reward" when
	// the sender was the faucet at the moment of the transfer — judged from the
	// address history, not from whichever wallet is the faucet now, so a
	// rotation never relabels the past — and "transfer" otherwise.
	//
	// Always set on rows from /transactions, so a missing field means a backend
	// too old to label, and only then should a client fall back to comparing
	// against /config's faucet_address. Were "not a reward" also an absent
	// field, a client could not tell the two apart.
	Type string `json:"type,omitempty"`
}

const (
	// TransactionTypeVolunteerReward is a transfer paid by the faucet.
	TransactionTypeVolunteerReward = "volunteer_reward"
	// TransactionTypeTransfer is any other transfer.
	TransactionTypeTransfer = "transfer"
)

type PonderTransactionsPage struct {
	Transactions []*PonderTransaction `json:"transactions"`
	Total        uint64               `json:"total"`
}

type PonderTransactionParties struct {
	ChainID int64  `json:"chain_id"`
	Hash    string `json:"hash"`
	From    string `json:"from"`
	To      string `json:"to"`
}
