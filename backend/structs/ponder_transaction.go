package structs

type PonderTransaction struct {
	Id        string `json:"id"`
	Hash      string `json:"hash"`
	Amount    string `json:"amount"`
	Timestamp uint64 `json:"timestamp"`
	From      string `json:"from"`
	To        string `json:"to"`
	Memo      string `json:"memo,omitempty"`
}

type PonderTransactionsPage struct {
	Transactions []*PonderTransaction `json:"transactions"`
	Total        uint64               `json:"total"`
}

type PonderTransactionParties struct {
	Hash string `json:"hash"`
	From string `json:"from"`
	To   string `json:"to"`
}
