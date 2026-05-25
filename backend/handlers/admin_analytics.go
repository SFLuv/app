package handlers

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	appdb "github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

const analyticsZeroAddress = "0x0000000000000000000000000000000000000000"

type analyticsRoleSet map[string]struct{}

type analyticsRoleIndex map[string][]*structs.AnalyticsWalletRoleRecord

type analyticsPeriodBucket struct {
	Key                string
	Label              string
	Unit               string
	Start              time.Time
	End                time.Time
	AllTime            bool
	ActiveUsers        map[string]struct{}
	ActiveWallets      map[string]struct{}
	Transactions       int
	TransactionVolume  *big.Int
	Rewards            *big.Int
	RewardCount        int
	Payments           *big.Int
	PaymentPairs       map[string]map[string]int
	Redemptions        *big.Int
	UniqueVolunteers   map[string]struct{}
	WeightedSpendSecs  *big.Int
	WeightedSpendValue *big.Int
	Events             int
}

type analyticsPayment struct {
	From      string
	Timestamp uint64
}

type analyticsReward struct {
	To        string
	Amount    *big.Int
	Timestamp uint64
}

func (p *PonderService) GetAdminAnalyticsDashboard(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	chainID := p.requestChainID(r)

	if err := p.SyncAnalyticsWalletRoleHistory(r.Context(), chainID); err != nil {
		p.logger.Logf("error syncing analytics wallet role history: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	roleHistory, err := p.appDB.GetAnalyticsWalletRoleHistory(r.Context())
	if err != nil {
		p.logger.Logf("error loading analytics wallet role history: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	roles := buildAnalyticsRoleIndex(roleHistory, chainID)

	transfers, err := p.db.GetAnalyticsTransfersSince(r.Context(), chainID, 0)
	if err != nil {
		p.logger.Logf("error loading analytics transfers: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	events, err := p.botDB.GetAnalyticsVolunteerEvents(r.Context())
	if err != nil {
		p.logger.Logf("error loading analytics bot events: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	periods := analyticsReportingPeriods(now)
	activity, err := p.appDB.GetAnalyticsUserActivity(r.Context(), time.Unix(0, 0).UTC())
	if err != nil {
		p.logger.Logf("error loading analytics user activity: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for day, users := range activity {
		activityTime := time.Unix(day, 0).UTC()
		for _, bucket := range periods {
			if bucket.contains(activityTime) {
				for userID := range users {
					bucket.ActiveUsers[userID] = struct{}{}
				}
			}
		}
	}

	paymentsBySender := make(map[string][]analyticsPayment)
	rewards := make([]analyticsReward, 0)
	for _, tx := range transfers {
		if tx == nil {
			continue
		}
		from := normalizeAnalyticsAddress(tx.From)
		to := normalizeAnalyticsAddress(tx.To)
		amount := parseAnalyticsBigInt(tx.Amount)
		if from == "" || to == "" || amount.Sign() <= 0 || from == analyticsZeroAddress || to == analyticsZeroAddress {
			continue
		}
		txTime := time.Unix(int64(tx.Timestamp), 0).UTC()
		fromRoles := roles.rolesAt(from, txTime)
		toRoles := roles.rolesAt(to, txTime)
		isReward := (fromRoles.has("admin") || fromRoles.has("faucet")) && toRoles.isUserWallet()
		isPayment := fromRoles.isUserWallet() && toRoles.has("merchant")
		isRedemption := fromRoles.has("merchant") && (toRoles.has("admin") || toRoles.has("zapper"))

		if isPayment {
			paymentsBySender[from] = append(paymentsBySender[from], analyticsPayment{From: from, Timestamp: tx.Timestamp})
		}
		if isReward {
			rewards = append(rewards, analyticsReward{To: to, Amount: new(big.Int).Set(amount), Timestamp: tx.Timestamp})
		}

		for _, bucket := range periods {
			if !bucket.contains(txTime) {
				continue
			}
			bucket.Transactions++
			bucket.TransactionVolume.Add(bucket.TransactionVolume, amount)
			bucket.ActiveWallets[from] = struct{}{}
			bucket.ActiveWallets[to] = struct{}{}
			if isReward {
				bucket.Rewards.Add(bucket.Rewards, amount)
				bucket.RewardCount++
				bucket.UniqueVolunteers[to] = struct{}{}
			}
			if isPayment {
				bucket.Payments.Add(bucket.Payments, amount)
				if bucket.PaymentPairs[from] == nil {
					bucket.PaymentPairs[from] = make(map[string]int)
				}
				bucket.PaymentPairs[from][to]++
			}
			if isRedemption {
				bucket.Redemptions.Add(bucket.Redemptions, amount)
			}
		}
	}

	for _, list := range paymentsBySender {
		sort.Slice(list, func(i, j int) bool { return list[i].Timestamp < list[j].Timestamp })
	}
	for _, reward := range rewards {
		payment, ok := nextAnalyticsPayment(paymentsBySender[reward.To], reward.Timestamp)
		if !ok {
			continue
		}
		seconds := int64(payment.Timestamp - reward.Timestamp)
		if seconds <= 0 {
			continue
		}
		rewardTime := time.Unix(int64(reward.Timestamp), 0).UTC()
		for _, bucket := range periods {
			if bucket.contains(rewardTime) {
				bucket.WeightedSpendSecs.Add(bucket.WeightedSpendSecs, new(big.Int).Mul(big.NewInt(seconds), reward.Amount))
				bucket.WeightedSpendValue.Add(bucket.WeightedSpendValue, reward.Amount)
			}
		}
	}

	for _, event := range events {
		eventTime := analyticsEventTime(event)
		if eventTime.IsZero() {
			continue
		}
		for _, bucket := range periods {
			if bucket.contains(eventTime) {
				bucket.Events++
			}
		}
	}

	periodRows := make([]*structs.AdminAnalyticsPeriod, 0, 7)
	for _, bucket := range periods[:7] {
		periodRows = append(periodRows, bucket.toPeriod())
	}
	trendRows := make([]*structs.AdminAnalyticsTrendPoint, 0, len(periods)-7)
	for _, bucket := range periods[7:] {
		trendRows = append(trendRows, bucket.toTrendPoint())
	}

	allTime := periods[6]
	circulating := new(big.Int).Sub(new(big.Int).Set(allTime.Rewards), allTime.Redemptions)
	if circulating.Sign() < 0 {
		circulating = big.NewInt(0)
	}

	response := structs.AdminAnalyticsResponse{
		GeneratedAt: now.Unix(),
		ChainID:     chainID,
		Summary: structs.AdminAnalyticsSummary{
			CurrentCirculatingSFLUVWei: analyticsBigIntString(circulating),
		},
		Periods:           periodRows,
		MonthlyTrend:      trendRows,
		MetricDefinitions: adminAnalyticsMetricDefinitions(),
		Glossary:          adminAnalyticsGlossary(),
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (p *PonderService) SyncAnalyticsWalletRoleHistory(ctx context.Context, chainID int64) error {
	walletOwners, err := p.appDB.GetAnalyticsWalletOwners(ctx)
	if err != nil {
		return err
	}
	merchantWallets, err := p.appDB.GetAnalyticsMerchantWallets(ctx)
	if err != nil {
		return err
	}
	return p.appDB.SyncAnalyticsWalletRoleHistory(ctx, chainID, buildAnalyticsWalletRoleCandidates(chainID, walletOwners, merchantWallets))
}

func (p *PonderService) SyncCurrentAnalyticsWalletRoleHistory(ctx context.Context) error {
	return p.SyncAnalyticsWalletRoleHistory(ctx, p.requestChainID(nil))
}

func buildAnalyticsWalletRoleCandidates(chainID int64, owners []*structs.AnalyticsWalletOwner, merchants []*structs.AnalyticsMerchantWallet) []appdb.AnalyticsWalletRoleCandidate {
	candidates := make([]appdb.AnalyticsWalletRoleCandidate, 0)
	appendCandidate := func(address string, role string, userID string, locationID int, source string) {
		address = normalizeAnalyticsAddress(address)
		if address == "" || address == analyticsZeroAddress {
			return
		}
		candidates = append(candidates, appdb.AnalyticsWalletRoleCandidate{Address: address, Role: role, ChainID: chainID, UserID: userID, LocationID: locationID, Source: source})
	}
	for _, owner := range owners {
		if owner == nil {
			continue
		}
		if owner.IsAdmin {
			appendCandidate(owner.Address, "admin", owner.UserID, 0, "users.is_admin")
		}
		if owner.IsMerchant {
			appendCandidate(owner.Address, "merchant", owner.UserID, 0, "users.is_merchant")
		}
	}
	for _, merchant := range merchants {
		if merchant == nil {
			continue
		}
		appendCandidate(merchant.Address, "merchant", merchant.OwnerID, merchant.LocationID, "merchant_location")
	}
	for _, address := range utils.MergeAddressLists(utils.ParseAddressList(os.Getenv("PAID_ADMIN_ADDRESSES")), os.Getenv("ADMIN_ADDRESS")) {
		appendCandidate(address, "admin", "", 0, "env.admin")
	}
	for _, address := range utils.MergeAddressLists(utils.ParseAddressList(os.Getenv("BOT_ADDRESS")), os.Getenv("FAUCET_ADDRESS"), os.Getenv("NEXT_PUBLIC_FAUCET_ADDRESS")) {
		appendCandidate(address, "faucet", "", 0, "env.faucet")
	}
	for _, address := range utils.MergeAddressLists(utils.ParseAddressList(os.Getenv("ZAPPER_ADDRESS")), os.Getenv("NEXT_PUBLIC_ZAPPER_ADDRESS")) {
		appendCandidate(address, "zapper", "", 0, "env.zapper")
	}
	return candidates
}

func buildAnalyticsRoleIndex(records []*structs.AnalyticsWalletRoleRecord, chainID int64) analyticsRoleIndex {
	index := make(analyticsRoleIndex)
	for _, record := range records {
		if record == nil || record.ChainID != chainID {
			continue
		}
		address := normalizeAnalyticsAddress(record.Address)
		if address == "" {
			continue
		}
		index[address] = append(index[address], record)
	}
	return index
}

func (index analyticsRoleIndex) rolesAt(address string, at time.Time) analyticsRoleSet {
	roles := make(analyticsRoleSet)
	unix := at.Unix()
	for _, record := range index[normalizeAnalyticsAddress(address)] {
		if record == nil || record.StartedAt > unix {
			continue
		}
		if record.EndedAt > 0 && record.EndedAt <= unix {
			continue
		}
		roles[record.Role] = struct{}{}
	}
	return roles
}

func (roles analyticsRoleSet) has(role string) bool {
	_, ok := roles[role]
	return ok
}

func (roles analyticsRoleSet) isUserWallet() bool {
	return !roles.has("admin") && !roles.has("merchant") && !roles.has("faucet") && !roles.has("zapper")
}

func newAnalyticsPeriod(key string, label string, unit string, start time.Time, end time.Time, allTime bool) *analyticsPeriodBucket {
	return &analyticsPeriodBucket{
		Key:                key,
		Label:              label,
		Unit:               unit,
		Start:              start,
		End:                end,
		AllTime:            allTime,
		ActiveUsers:        make(map[string]struct{}),
		ActiveWallets:      make(map[string]struct{}),
		TransactionVolume:  big.NewInt(0),
		Rewards:            big.NewInt(0),
		Payments:           big.NewInt(0),
		PaymentPairs:       make(map[string]map[string]int),
		Redemptions:        big.NewInt(0),
		UniqueVolunteers:   make(map[string]struct{}),
		WeightedSpendSecs:  big.NewInt(0),
		WeightedSpendValue: big.NewInt(0),
	}
}

func analyticsReportingPeriods(now time.Time) []*analyticsPeriodBucket {
	now = now.UTC()
	weekStart := startOfAnalyticsWeek(now)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	periods := []*analyticsPeriodBucket{
		newAnalyticsPeriod("current_week", "Current Week", "week", weekStart, weekStart.AddDate(0, 0, 7), false),
		newAnalyticsPeriod("previous_week", "Previous Week", "week", weekStart.AddDate(0, 0, -7), weekStart, false),
		newAnalyticsPeriod("current_month", "Current Month", "month", monthStart, monthStart.AddDate(0, 1, 0), false),
		newAnalyticsPeriod("previous_month", "Previous Month", "month", monthStart.AddDate(0, -1, 0), monthStart, false),
		newAnalyticsPeriod("current_year", "Current Year", "year", yearStart, yearStart.AddDate(1, 0, 0), false),
		newAnalyticsPeriod("previous_year", "Previous Year", "year", yearStart.AddDate(-1, 0, 0), yearStart, false),
		newAnalyticsPeriod("all_time", "All Time", "all_time", time.Unix(0, 0).UTC(), now.Add(time.Second), true),
	}
	for i := 11; i >= 0; i-- {
		start := monthStart.AddDate(0, -i, 0)
		periods = append(periods, newAnalyticsPeriod(start.Format("2006-01"), start.Format("Jan 2006"), "month", start, start.AddDate(0, 1, 0), false))
	}
	return periods
}

func startOfAnalyticsWeek(value time.Time) time.Time {
	date := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	return date.AddDate(0, 0, -int(date.Weekday()))
}

func (bucket *analyticsPeriodBucket) contains(value time.Time) bool {
	if bucket == nil {
		return false
	}
	value = value.UTC()
	return !value.Before(bucket.Start) && value.Before(bucket.End)
}

func (bucket *analyticsPeriodBucket) toPeriod() *structs.AdminAnalyticsPeriod {
	return &structs.AdminAnalyticsPeriod{Key: bucket.Key, Label: bucket.Label, Unit: bucket.Unit, StartAt: bucket.Start.Unix(), EndAt: bucket.End.Unix() - 1, Metrics: bucket.metrics()}
}

func (bucket *analyticsPeriodBucket) toTrendPoint() *structs.AdminAnalyticsTrendPoint {
	return &structs.AdminAnalyticsTrendPoint{Key: bucket.Key, Label: bucket.Label, StartAt: bucket.Start.Unix(), EndAt: bucket.End.Unix() - 1, Metrics: bucket.metrics()}
}

func (bucket *analyticsPeriodBucket) metrics() []*structs.AdminAnalyticsMetricValue {
	return []*structs.AdminAnalyticsMetricValue{
		{MetricKey: "active_users", Label: "Active Users", Kind: "count", Count: len(bucket.ActiveUsers)},
		{MetricKey: "active_wallets", Label: "Active Wallets", Kind: "count", Count: len(bucket.ActiveWallets)},
		{MetricKey: "transactions", Label: "Transactions", Kind: "count", Count: bucket.Transactions},
		{MetricKey: "transaction_volume", Label: "Transaction Volume", Kind: "wei", Wei: analyticsBigIntString(bucket.TransactionVolume)},
		{MetricKey: "rewards", Label: "Rewards", Kind: "wei", Wei: analyticsBigIntString(bucket.Rewards)},
		{MetricKey: "total_payments", Label: "Total Payments", Kind: "wei", Wei: analyticsBigIntString(bucket.Payments)},
		{MetricKey: "total_sfluv_distributed", Label: "Total SFLUV Distributed", Kind: "wei", Wei: analyticsBigIntString(bucket.Rewards)},
		{MetricKey: "usage_percentage", Label: "Usage Percentage", Kind: "percent", Percent: analyticsRatioPercentBig(bucket.Payments, bucket.Rewards)},
		{MetricKey: "unique_volunteers", Label: "Unique Volunteers", Kind: "count", Count: len(bucket.UniqueVolunteers)},
		{MetricKey: "volunteer_frequency", Label: "Volunteer Frequency", Kind: "decimal", Decimal: analyticsAverageFloat(bucket.RewardCount, len(bucket.UniqueVolunteers))},
		{MetricKey: "repeat_business", Label: "Repeat Business", Kind: "count", Count: analyticsRepeatBusiness(bucket.PaymentPairs)},
		{MetricKey: "value_weighted_average_time_to_spend", Label: "Value-Weighted Average Time to Spend", Kind: "seconds", Seconds: analyticsWeightedAverageSeconds(bucket.WeightedSpendSecs, bucket.WeightedSpendValue)},
		{MetricKey: "event_frequency", Label: "Event Frequency", Kind: "decimal", Decimal: float64(bucket.Events)},
	}
}

func analyticsRepeatBusiness(pairs map[string]map[string]int) int {
	repeat := make(map[string]struct{})
	for userWallet, merchantCounts := range pairs {
		for _, count := range merchantCounts {
			if count >= 2 {
				repeat[userWallet] = struct{}{}
			}
		}
	}
	return len(repeat)
}

func analyticsWeightedAverageSeconds(total *big.Int, value *big.Int) int64 {
	if total == nil || value == nil || value.Sign() <= 0 {
		return 0
	}
	return new(big.Int).Div(new(big.Int).Set(total), value).Int64()
}

func analyticsAverageFloat(total int, count int) float64 {
	if total <= 0 || count <= 0 {
		return 0
	}
	return math.Round((float64(total)/float64(count))*10) / 10
}

func nextAnalyticsPayment(payments []analyticsPayment, after uint64) (analyticsPayment, bool) {
	for _, payment := range payments {
		if payment.Timestamp > after {
			return payment, true
		}
	}
	return analyticsPayment{}, false
}

func normalizeAnalyticsAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

func parseAnalyticsBigInt(value string) *big.Int {
	amount, ok := new(big.Int).SetString(strings.TrimSpace(value), 10)
	if !ok {
		return big.NewInt(0)
	}
	return amount
}

func analyticsBigIntString(value *big.Int) string {
	if value == nil {
		return "0"
	}
	return value.String()
}

func analyticsRatioPercentBig(part *big.Int, total *big.Int) float64 {
	if part == nil || total == nil || total.Sign() <= 0 {
		return 0
	}
	partFloat, _ := new(big.Float).SetInt(part).Float64()
	totalFloat, _ := new(big.Float).SetInt(total).Float64()
	if totalFloat == 0 {
		return 0
	}
	return math.Round((partFloat/totalFloat)*1000) / 10
}

func analyticsEventTime(event *structs.AnalyticsVolunteerEvent) time.Time {
	if event == nil {
		return time.Time{}
	}
	if event.StartAt > 0 {
		return time.Unix(event.StartAt, 0).UTC()
	}
	if event.Expiration > 0 {
		return time.Unix(event.Expiration, 0).UTC()
	}
	return time.Time{}
}

func adminAnalyticsMetricDefinitions() []structs.AdminAnalyticsDefinition {
	return []structs.AdminAnalyticsDefinition{
		{Key: "current_circulating_sfluv", Label: "Current Circulating SFLUV", Definition: "All-time total SFLUV distributed through rewards minus all-time redemptions."},
		{Key: "active_users", Label: "Active Users", Definition: "Users with an observed authenticated web or mobile session in the period."},
		{Key: "active_wallets", Label: "Active Wallets", Definition: "Wallet addresses with at least one confirmed SFLUV transfer in the period."},
		{Key: "transactions", Label: "Transactions", Definition: "Confirmed SFLUV blockchain transfers in the period."},
		{Key: "transaction_volume", Label: "Transaction Volume", Definition: "Aggregate SFLUV value transferred in confirmed SFLUV transfers in the period."},
		{Key: "rewards", Label: "Rewards", Definition: "Aggregate SFLUV value sent from admin or faucet wallets to user wallets in the period."},
		{Key: "total_payments", Label: "Total Payments", Definition: "Aggregate SFLUV value sent from user wallets to merchant wallets in the period."},
		{Key: "total_sfluv_distributed", Label: "Total SFLUV Distributed", Definition: "Same calculation as rewards: aggregate admin/faucet to user-wallet transfers in the period."},
		{Key: "usage_percentage", Label: "Usage Percentage", Definition: "Payments divided by rewards for the period."},
		{Key: "unique_volunteers", Label: "Unique Volunteers", Definition: "Unique user wallets that received at least one reward in the period."},
		{Key: "volunteer_frequency", Label: "Volunteer Frequency", Definition: "Reward count divided by unique volunteer wallets in the period."},
		{Key: "repeat_business", Label: "Repeat Business", Definition: "User wallets that made at least two payments to the same merchant wallet in the period."},
		{Key: "value_weighted_average_time_to_spend", Label: "Value-Weighted Average Time to Spend", Definition: "Sum of reward time to next payment multiplied by reward value, divided by aggregate reward value."},
		{Key: "event_frequency", Label: "Event Frequency", Definition: "Bot DB events occurring in the period."},
	}
}

func adminAnalyticsGlossary() []structs.AdminAnalyticsDefinition {
	return []structs.AdminAnalyticsDefinition{
		{Key: "users", Label: "Users", Definition: "An entry in the users table with one or more attached wallets. Users are platform specific."},
		{Key: "wallets", Label: "Wallets", Definition: "Any address that has engaged with SFLUV by sending or receiving a transfer."},
		{Key: "admin_wallet", Label: "Admin Wallet", Definition: "A wallet associated with an admin user, with historical wallet role records respected by timestamp and chain."},
		{Key: "merchant_wallet", Label: "Merchant Wallet", Definition: "A wallet associated with a merchant user or merchant location, with historical wallet role records respected by timestamp and chain."},
		{Key: "faucet_wallet", Label: "Faucet Wallet", Definition: "A backend-controlled payout wallet, with historical wallet role records respected by timestamp and chain."},
		{Key: "zapper_wallet", Label: "Zapper Wallet", Definition: "The configured zapper wallet, with historical wallet role records respected by timestamp and chain."},
		{Key: "user_wallet", Label: "User Wallet", Definition: "Any wallet not classified as admin, merchant, faucet, or zapper at the transfer timestamp."},
		{Key: "payment", Label: "Payment", Definition: "A transfer from a user wallet to a merchant wallet."},
		{Key: "reward", Label: "Reward", Definition: "A transfer from an admin or faucet wallet to a user wallet."},
		{Key: "redemption", Label: "Redemption", Definition: "A transfer from a merchant wallet to an admin or zapper wallet."},
	}
}
