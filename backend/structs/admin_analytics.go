package structs

type AnalyticsWalletOwner struct {
	UserID     string `json:"user_id"`
	Address    string `json:"address"`
	IsAdmin    bool   `json:"is_admin"`
	IsMerchant bool   `json:"is_merchant"`
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
	TotalActiveUsers                 int     `json:"total_active_users"`
	DailyActiveUsers                 int     `json:"daily_active_users"`
	MonthlyActiveUsers               int     `json:"monthly_active_users"`
	PreviousMonthlyActiveUsers       int     `json:"previous_monthly_active_users"`
	MonthlyActiveUserChangePercent   float64 `json:"monthly_active_user_change_percent"`
	MonthlyTransactionVolumeWei      string  `json:"monthly_transaction_volume_wei"`
	TotalDistributedWei              string  `json:"total_distributed_wei"`
	DistributedSpentAtMerchantsWei   string  `json:"distributed_spent_at_merchants_wei"`
	TokenRedeemedWei                 string  `json:"token_redeemed_wei"`
	TokenUnusedWei                   string  `json:"token_unused_wei"`
	TokenRedeemedPercent             float64 `json:"token_redeemed_percent"`
	AverageCommunityProjectCostWei   string  `json:"average_community_project_cost_wei"`
	MerchantCustomerWallets          int     `json:"merchant_customer_wallets"`
	MerchantRepeatCustomerWallets    int     `json:"merchant_repeat_customer_wallets"`
	MerchantRepeatCustomerPercent    float64 `json:"merchant_repeat_customer_percent"`
	AverageSecondsToUseDistribution  int64   `json:"average_seconds_to_use_distribution"`
	AverageVolunteerEventsPerWeek    float64 `json:"average_volunteer_events_per_week"`
	AverageVolunteerEventsPerMonth   float64 `json:"average_volunteer_events_per_month"`
	AverageVolunteerEventsPerYear    float64 `json:"average_volunteer_events_per_year"`
	AverageUniqueEarnersPerWeek      float64 `json:"average_unique_earners_per_week"`
	AverageUniqueEarnersPerMonth     float64 `json:"average_unique_earners_per_month"`
	AverageUniqueEarnersPerYear      float64 `json:"average_unique_earners_per_year"`
	VolunteerParticipationCount      int     `json:"volunteer_participation_count"`
	VolunteerUniqueEarners           int     `json:"volunteer_unique_earners"`
	VolunteerRepeatEarners           int     `json:"volunteer_repeat_earners"`
	VolunteerRepeatParticipationRate float64 `json:"volunteer_repeat_participation_rate"`
	VolunteerEvents                  int     `json:"volunteer_events"`
	EventCodeRedemptionPercent       float64 `json:"event_code_redemption_percent"`
}

type AdminAnalyticsMonthlyPoint struct {
	Key                           string `json:"key"`
	Label                         string `json:"label"`
	ActiveUsers                   int    `json:"active_users"`
	TransactionVolumeWei          string `json:"transaction_volume_wei"`
	DistributedWei                string `json:"distributed_wei"`
	MerchantSpendWei              string `json:"merchant_spend_wei"`
	MerchantCustomerWallets       int    `json:"merchant_customer_wallets"`
	MerchantRepeatCustomerWallets int    `json:"merchant_repeat_customer_wallets"`
	VolunteerEvents               int    `json:"volunteer_events"`
	UniqueEarners                 int    `json:"unique_earners"`
	AverageProjectCostWei         string `json:"average_project_cost_wei"`
}

type AdminAnalyticsDailyPoint struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	ActiveUsers int    `json:"active_users"`
}

type AdminAnalyticsDefinition struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Definition string `json:"definition"`
}

type AdminAnalyticsResponse struct {
	GeneratedAt             int64                         `json:"generated_at"`
	ConfiguredPaidAddresses []string                      `json:"configured_paid_addresses"`
	Summary                 AdminAnalyticsSummary         `json:"summary"`
	Monthly                 []*AdminAnalyticsMonthlyPoint `json:"monthly"`
	Daily                   []*AdminAnalyticsDailyPoint   `json:"daily"`
	MetricDefinitions       []AdminAnalyticsDefinition    `json:"metric_definitions"`
}
