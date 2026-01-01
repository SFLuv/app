package structs

type AlchemyHook struct {
	CreatedAt string       `json:"createdAt"`
	Event     AlchemyEvent `json:"event"`
	Id        string       `json:"id"`
	Type      string       `json:"type"`
	WebhookId string       `json:"webhookId"`
}

type AlchemyEvent struct {
	Activity []AlchemyTx `json:"activity"`
	Network  string      `json:"network"`
}

type AlchemyTx struct {
	Asset       string          `json:"asset"`
	BlockNum    string          `json:"blockNum"`
	Category    string          `json:"category"`
	FromAddress string          `json:"fromAddress"`
	ToAddress   string          `json:"toAddress"`
	Value       float64         `json:"value"`
	Hash        string          `json:"hash"`
	RawContract AlchemyContract `json:"rawContract"`
	Log         AlchemyLog      `json:"log"`
}

type AlchemyContract struct {
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
}

type AlchemyLog struct {
	Topics []string `json:"topics"`
	Data   string   `json:"data"`
}

type AlchemyCreateWebhookRequest struct {
}
