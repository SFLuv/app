package structs

type SubscriptionType string

const (
	MerchantSubscription SubscriptionType = "merchant"
)

type PonderSubscriptionRequest struct {
	Address string `json:"address"`
	Email   string `json:"email"`
}

type PonderSubscription struct {
	Id      int              `json:"id"`
	Address string           `json:"address"`
	Type    SubscriptionType `json:"type"`
	Owner   string           `json:"owner"`
	Data    []byte           `json:"data"`
}

type PonderSubscriptionServerRequest struct {
	Id      int    `json:"id"`
	Address string `json:"address"`
	Url     string `json:"url"`
}

type PonderHookData struct {
	To     string `json:"to"`
	From   string `json:"from"`
	Hash   string `json:"hash"`
	Amount string `json:"amount"`
}
