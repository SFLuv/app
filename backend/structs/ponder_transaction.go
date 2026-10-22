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
	// Type is set by the backend when it knows what a transfer was. Today that
	// is "volunteer_reward": the sender was the faucet at the moment of the
	// transfer, judged from the address history rather than from whichever
	// wallet is the faucet now, so a rotation never relabels the past.
	// Empty means an ordinary transfer, or a backend too old to say.
	Type string `json:"type,omitempty"`
}

// TransactionTypeVolunteerReward is a transfer paid by the faucet.
const TransactionTypeVolunteerReward = "volunteer_reward"

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
