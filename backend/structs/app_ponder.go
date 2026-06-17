package structs

type SubscriptionType string

const (
	MerchantSubscription SubscriptionType = "merchant"
	PushSubscription     SubscriptionType = "push"
)

type PonderSubscriptionRequest struct {
	Address string `json:"address"`
	Email   string `json:"email"`
}

type PushSubscriptionSyncRequest struct {
	Token             string   `json:"token"`
	Addresses         []string `json:"addresses"`
	InstallationID    string   `json:"installation_id,omitempty"`
	Enabled           *bool    `json:"enabled,omitempty"`
	PreferenceEnabled *bool    `json:"preference_enabled,omitempty"`
	DeviceRegistered  *bool    `json:"device_registered,omitempty"`
}

type PonderSubscription struct {
	Id      int              `json:"id"`
	Address string           `json:"address"`
	Type    SubscriptionType `json:"type"`
	Owner   string           `json:"owner"`
	Data    []byte           `json:"data"`
}

type MobilePushSubscription struct {
	Id                 int              `json:"id"`
	Owner              string           `json:"owner"`
	Token              string           `json:"token"`
	Address            string           `json:"address"`
	Type               SubscriptionType `json:"type"`
	Data               []byte           `json:"data,omitempty"`
	Active             bool             `json:"active"`
	PreferenceEnabled  bool             `json:"preference_enabled"`
	DeviceRegistered   bool             `json:"device_registered"`
	InstallationIDHash string           `json:"-"`
	PonderHookId       *int             `json:"ponder_hook_id,omitempty"`
}

type PonderSubscriptionServerRequest struct {
	Id      int    `json:"id"`
	Address string `json:"address"`
	Url     string `json:"url"`
}

type PonderHookData struct {
	ChainID int64  `json:"chain_id"`
	To      string `json:"to"`
	From    string `json:"from"`
	Hash    string `json:"hash"`
	Amount  string `json:"amount"`
}
