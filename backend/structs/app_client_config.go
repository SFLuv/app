package structs

type ClientNativeCurrency struct {
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

type ClientExplorerConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type ClientChainConfig struct {
	ID             int                  `json:"id"`
	Name           string               `json:"name"`
	NativeCurrency ClientNativeCurrency `json:"native_currency"`
	RPCURL         string               `json:"rpc_url"`
	EngineRPCURL   string               `json:"engine_rpc_url"`
	EngineWSURL    string               `json:"engine_ws_url,omitempty"`
	Explorer       ClientExplorerConfig `json:"explorer"`
}

type ClientTokenConfig struct {
	Standard string `json:"standard"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	ChainID  int    `json:"chain_id"`
	Decimals int    `json:"decimals"`
}

type ClientAccountConfig struct {
	ChainID               int    `json:"chain_id"`
	EntryPointAddress     string `json:"entrypoint_address"`
	AccountFactoryAddress string `json:"account_factory_address"`
	PaymasterAddress      string `json:"paymaster_address"`
	PaymasterType         string `json:"paymaster_type"`
}

type ClientCommunityConfig struct {
	Name           string `json:"name"`
	Alias          string `json:"alias"`
	CustomDomain   string `json:"custom_domain"`
	Logo           string `json:"logo,omitempty"`
	ProfileAddress string `json:"profile_address,omitempty"`
}

type ClientURLConfig struct {
	AppOrigin                   string `json:"app_origin"`
	Backend                     string `json:"backend"`
	CitizenWalletConfigLocation string `json:"citizen_wallet_config_location,omitempty"`
	IPFS                        string `json:"ipfs,omitempty"`
}

type ClientFeatureConfig struct {
	MigrationBanner         bool `json:"migration_banner"`
	SendsEnabled            bool `json:"sends_enabled"`
	RedemptionsEnabled      bool `json:"redemptions_enabled"`
	WorkflowPayoutsEnabled  bool `json:"workflow_payouts_enabled"`
	MerchantPaymentsEnabled bool `json:"merchant_payments_enabled"`
}

type ClientMigrationConfig struct {
	State            string  `json:"state"`
	Message          string  `json:"message"`
	CutoverStartedAt *string `json:"cutover_started_at"`
}

type ClientConfigSource struct {
	Provider     string `json:"provider"`
	FallbackUsed bool   `json:"fallback_used"`
	FetchedAt    string `json:"fetched_at"`
}

type ClientConfigResponse struct {
	SchemaVersion  int                            `json:"schema_version"`
	ConfigVersion  string                         `json:"config_version"`
	Environment    string                         `json:"environment"`
	ActiveChainID  int                            `json:"active_chain_id"`
	LegacyChainIDs []int                          `json:"legacy_chain_ids"`
	Community      ClientCommunityConfig          `json:"community"`
	Chains         map[string]ClientChainConfig   `json:"chains"`
	Tokens         map[string]ClientTokenConfig   `json:"tokens"`
	Accounts       map[string]ClientAccountConfig `json:"accounts"`
	URLs           ClientURLConfig                `json:"urls"`
	Features       ClientFeatureConfig            `json:"features"`
	Migration      ClientMigrationConfig          `json:"migration"`
	Source         ClientConfigSource             `json:"source"`
}

type ClientVersionInfo struct {
	Version string `json:"version"`
	Build   int    `json:"build"`
}

type ClientVersionFeatures struct {
	DynamicConfigRequired bool `json:"dynamic_config_required"`
	CeloRequired          bool `json:"celo_required"`
}

type ClientVersionResponse struct {
	SchemaVersion int                   `json:"schema_version"`
	ServerTime    string                `json:"server_time"`
	ConfigVersion string                `json:"config_version"`
	Platform      string                `json:"platform"`
	Status        string                `json:"status"`
	Minimum       ClientVersionInfo     `json:"minimum"`
	Recommended   ClientVersionInfo     `json:"recommended"`
	Current       ClientVersionInfo     `json:"current"`
	ForceUpdate   bool                  `json:"force_update"`
	Maintenance   bool                  `json:"maintenance"`
	UpdateURL     string                `json:"update_url"`
	Message       string                `json:"message"`
	Features      ClientVersionFeatures `json:"features"`
}
