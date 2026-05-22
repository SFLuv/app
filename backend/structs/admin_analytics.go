package structs

type AnalyticsWalletOwner struct {
	UserID     string `json:"user_id"`
	Address    string `json:"address"`
	IsAdmin    bool   `json:"is_admin"`
	IsMerchant bool   `json:"is_merchant"`
}

type AnalyticsWalletRoleRecord struct {
	Address    string `json:"address"`
	Role       string `json:"role"`
	ChainID    int64  `json:"chain_id"`
	UserID     string `json:"user_id"`
	LocationID int    `json:"location_id"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    int64  `json:"ended_at"`
}

type AnalyticsMerchantWallet struct {
	OwnerID      string `json:"owner_id"`
	LocationID   int    `json:"location_id"`
	LocationName string `json:"location_name"`
	Address      string `json:"address"`
}

type AnalyticsTransfer struct {
	Hash      string `json:"hash"`
	Amount    string `json:"amount"`
	Timestamp uint64 `json:"timestamp"`
	From      string `json:"from"`
	To        string `json:"to"`
}

type AnalyticsAddressBalance struct {
	Address string `json:"address"`
	Balance string `json:"balance"`
}

type AnalyticsVolunteerEvent struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Amount          int64    `json:"amount"`
	StartAt         int64    `json:"start_at"`
	Expiration      int64    `json:"expiration"`
	CodeCount       int      `json:"code_count"`
	RedeemedCount   int      `json:"redeemed_count"`
	UniqueEarners   int      `json:"unique_earners"`
	EarnerAddresses []string `json:"-"`
}

type AnalyticsWorkflowCost struct {
	ID          string `json:"id"`
	CompletedAt int64  `json:"completed_at"`
	CostWei     string `json:"cost_wei"`
}

type AdminAnalyticsSummary struct {
	CurrentCirculatingSFLUVWei string `json:"current_circulating_sfluv_wei"`
}

type AdminAnalyticsMetricValue struct {
	MetricKey string  `json:"metric_key"`
	Label     string  `json:"label"`
	Kind      string  `json:"kind"`
	Wei       string  `json:"wei,omitempty"`
	Count     int     `json:"count,omitempty"`
	Seconds   int64   `json:"seconds,omitempty"`
	Percent   float64 `json:"percent,omitempty"`
	Decimal   float64 `json:"decimal,omitempty"`
}

type AdminAnalyticsPeriod struct {
	Key     string                       `json:"key"`
	Label   string                       `json:"label"`
	Unit    string                       `json:"unit"`
	StartAt int64                        `json:"start_at"`
	EndAt   int64                        `json:"end_at"`
	Metrics []*AdminAnalyticsMetricValue `json:"metrics"`
}

type AdminAnalyticsTrendPoint struct {
	Key     string                       `json:"key"`
	Label   string                       `json:"label"`
	StartAt int64                        `json:"start_at"`
	EndAt   int64                        `json:"end_at"`
	Metrics []*AdminAnalyticsMetricValue `json:"metrics"`
}

type AdminAnalyticsDefinition struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Definition string `json:"definition"`
}

type AdminAnalyticsResponse struct {
	GeneratedAt       int64                       `json:"generated_at"`
	ChainID           int64                       `json:"chain_id"`
	Summary           AdminAnalyticsSummary       `json:"summary"`
	Periods           []*AdminAnalyticsPeriod     `json:"periods"`
	MonthlyTrend      []*AdminAnalyticsTrendPoint `json:"monthly_trend"`
	MetricDefinitions []AdminAnalyticsDefinition  `json:"metric_definitions"`
	Glossary          []AdminAnalyticsDefinition  `json:"glossary"`
}
