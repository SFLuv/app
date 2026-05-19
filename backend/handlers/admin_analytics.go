package handlers

import (
	"encoding/json"
	"math"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

type adminAnalyticsOwnerMeta struct {
	UserID     string
	IsAdmin    bool
	IsMerchant bool
}

type adminAnalyticsMonthBucket struct {
	Key               string
	Label             string
	Start             time.Time
	ActiveUsers       map[string]struct{}
	TransactionVolume *big.Int
	Distributed       *big.Int
	MerchantSpend     *big.Int
	MerchantCustomers map[string]int
	VolunteerEvents   int
	UniqueEarners     map[string]struct{}
	ProjectCost       *big.Int
	ProjectCount      int
}

type adminAnalyticsDayBucket struct {
	Key         string
	Label       string
	ActiveUsers map[string]struct{}
}

func (p *PonderService) GetAdminAnalyticsDashboard(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	walletOwners, err := p.appDB.GetAnalyticsWalletOwners(r.Context())
	if err != nil {
		p.logger.Logf("error loading analytics wallet owners: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	merchantWallets, err := p.appDB.GetAnalyticsMerchantWallets(r.Context())
	if err != nil {
		p.logger.Logf("error loading analytics merchant wallets: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	totalActiveUsers, err := p.appDB.GetAnalyticsActiveUserCount(r.Context())
	if err != nil {
		p.logger.Logf("error loading analytics active user count: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	workflowCosts, err := p.appDB.GetAnalyticsWorkflowCosts(r.Context())
	if err != nil {
		p.logger.Logf("error loading analytics workflow costs: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	volunteerEvents, err := p.botDB.GetAnalyticsVolunteerEvents(r.Context())
	if err != nil {
		p.logger.Logf("error loading analytics volunteer events: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	volunteerParticipation, err := p.botDB.GetAnalyticsVolunteerParticipationCounts(r.Context())
	if err != nil {
		p.logger.Logf("error loading analytics volunteer participation: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	transfers, err := p.db.GetAnalyticsTransfersSince(r.Context(), 0)
	if err != nil {
		p.logger.Logf("error loading analytics transfers: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ownerByAddress := make(map[string]adminAnalyticsOwnerMeta, len(walletOwners))
	registeredNonAdminAddresses := make([]string, 0, len(walletOwners))
	for _, row := range walletOwners {
		if row == nil {
			continue
		}
		address := normalizeAnalyticsAddress(row.Address)
		if address == "" {
			continue
		}
		meta := adminAnalyticsOwnerMeta{
			UserID:     row.UserID,
			IsAdmin:    row.IsAdmin,
			IsMerchant: row.IsMerchant,
		}
		ownerByAddress[address] = meta
		if !row.IsAdmin {
			registeredNonAdminAddresses = append(registeredNonAdminAddresses, address)
		}
	}

	merchantByAddress := make(map[string]*structs.AnalyticsMerchantWallet, len(merchantWallets))
	for _, wallet := range merchantWallets {
		if wallet == nil {
			continue
		}
		address := normalizeAnalyticsAddress(wallet.Address)
		if address != "" {
			merchantByAddress[address] = wallet
		}
	}

	paidAddresses := utils.MergeAddressLists(
		utils.ParseAddressList(os.Getenv("PAID_ADMIN_ADDRESSES")),
		os.Getenv("ADMIN_ADDRESS"),
		os.Getenv("BOT_ADDRESS"),
		os.Getenv("NEXT_PUBLIC_FAUCET_ADDRESS"),
	)
	paidAddressSet := analyticsSetFromList(paidAddresses)

	balances, err := p.db.GetAnalyticsAddressBalances(r.Context(), registeredNonAdminAddresses)
	if err != nil {
		p.logger.Logf("error loading analytics balances: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	registeredNonAdminBalance := big.NewInt(0)
	for _, balance := range balances {
		if balance == nil {
			continue
		}
		amount := parseAnalyticsBigInt(balance.Balance)
		if amount.Sign() > 0 {
			registeredNonAdminBalance.Add(registeredNonAdminBalance, amount)
		}
	}

	monthlyOrder := make([]string, 0, 12)
	monthly := make(map[string]*adminAnalyticsMonthBucket, 12)
	for i := 11; i >= 0; i-- {
		start := monthStart.AddDate(0, -i, 0)
		key := start.Format("2006-01")
		monthlyOrder = append(monthlyOrder, key)
		monthly[key] = &adminAnalyticsMonthBucket{
			Key:               key,
			Label:             start.Format("Jan 2006"),
			Start:             start,
			ActiveUsers:       make(map[string]struct{}),
			TransactionVolume: big.NewInt(0),
			Distributed:       big.NewInt(0),
			MerchantSpend:     big.NewInt(0),
			MerchantCustomers: make(map[string]int),
			UniqueEarners:     make(map[string]struct{}),
			ProjectCost:       big.NewInt(0),
		}
	}

	dailyOrder := make([]string, 0, 30)
	daily := make(map[string]*adminAnalyticsDayBucket, 30)
	for i := 29; i >= 0; i-- {
		start := dayStart.AddDate(0, 0, -i)
		key := start.Format("2006-01-02")
		dailyOrder = append(dailyOrder, key)
		daily[key] = &adminAnalyticsDayBucket{
			Key:         key,
			Label:       start.Format("Jan 2"),
			ActiveUsers: make(map[string]struct{}),
		}
	}

	totalDistributed := big.NewInt(0)
	distributedSpentAtMerchants := big.NewInt(0)
	merchantCustomers := make(map[string]int)
	monthlyActiveNow := make(map[string]struct{})
	monthlyActivePrevious := make(map[string]struct{})
	dailyActiveNow := make(map[string]struct{})
	outgoingByAddress := make(map[string][]*structs.AnalyticsTransfer)

	for _, tx := range transfers {
		if tx == nil {
			continue
		}
		from := normalizeAnalyticsAddress(tx.From)
		to := normalizeAnalyticsAddress(tx.To)
		amount := parseAnalyticsBigInt(tx.Amount)
		if from == "" || to == "" || amount.Sign() <= 0 {
			continue
		}

		outgoingByAddress[from] = append(outgoingByAddress[from], tx)
		txTime := time.Unix(int64(tx.Timestamp), 0).UTC()

		monthKey := txTime.Format("2006-01")
		month := monthly[monthKey]
		if month != nil {
			month.TransactionVolume.Add(month.TransactionVolume, amount)
		}

		addActiveAnalyticsUser(ownerByAddress, from, txTime, month, daily, monthlyActiveNow, monthlyActivePrevious, dailyActiveNow, monthStart, dayStart)
		addActiveAnalyticsUser(ownerByAddress, to, txTime, month, daily, monthlyActiveNow, monthlyActivePrevious, dailyActiveNow, monthStart, dayStart)

		if _, ok := paidAddressSet[from]; ok {
			totalDistributed.Add(totalDistributed, amount)
			if month != nil {
				month.Distributed.Add(month.Distributed, amount)
			}
		}

		merchantWallet := merchantByAddress[to]
		if merchantWallet != nil {
			fromMeta, fromKnown := ownerByAddress[from]
			if fromKnown && fromMeta.UserID == merchantWallet.OwnerID {
				continue
			}
			if _, paid := paidAddressSet[from]; paid {
				continue
			}
			customerKey := from
			if fromKnown && fromMeta.UserID != "" {
				customerKey = fromMeta.UserID
			}
			merchantCustomers[customerKey]++
			distributedSpentAtMerchants.Add(distributedSpentAtMerchants, amount)
			if month != nil {
				month.MerchantSpend.Add(month.MerchantSpend, amount)
				month.MerchantCustomers[customerKey]++
			}
		}
	}

	latencyTotal := int64(0)
	latencyCount := int64(0)
	for _, tx := range transfers {
		if tx == nil {
			continue
		}
		from := normalizeAnalyticsAddress(tx.From)
		to := normalizeAnalyticsAddress(tx.To)
		if _, ok := paidAddressSet[from]; !ok {
			continue
		}
		for _, outgoing := range outgoingByAddress[to] {
			if outgoing == nil || outgoing.Timestamp <= tx.Timestamp {
				continue
			}
			outgoingTo := normalizeAnalyticsAddress(outgoing.To)
			if outgoingTo == to {
				continue
			}
			latencyTotal += int64(outgoing.Timestamp - tx.Timestamp)
			latencyCount++
			break
		}
	}

	tokenUnused := minAnalyticsBigInt(registeredNonAdminBalance, totalDistributed)
	tokenRedeemed := new(big.Int).Sub(new(big.Int).Set(totalDistributed), tokenUnused)
	if tokenRedeemed.Sign() < 0 {
		tokenRedeemed = big.NewInt(0)
	}

	totalProjectCost := big.NewInt(0)
	projectCount := 0
	for _, workflow := range workflowCosts {
		if workflow == nil {
			continue
		}
		cost := parseAnalyticsBigInt(workflow.CostWei)
		if cost.Sign() <= 0 {
			continue
		}
		totalProjectCost.Add(totalProjectCost, cost)
		projectCount++
		if workflow.CompletedAt > 0 {
			completedAt := time.Unix(workflow.CompletedAt, 0).UTC()
			if month := monthly[completedAt.Format("2006-01")]; month != nil {
				month.ProjectCost.Add(month.ProjectCost, cost)
				month.ProjectCount++
			}
		}
	}

	totalEventCodes := 0
	totalRedeemedCodes := 0
	eventPeriodStart := time.Time{}
	eventPeriodEnd := time.Time{}
	weeklyEarners := map[string]map[string]struct{}{}
	monthlyEarners := map[string]map[string]struct{}{}
	yearlyEarners := map[string]map[string]struct{}{}
	for _, event := range volunteerEvents {
		if event == nil {
			continue
		}
		eventTime := analyticsEventTime(event)
		if !eventTime.IsZero() {
			if eventPeriodStart.IsZero() || eventTime.Before(eventPeriodStart) {
				eventPeriodStart = eventTime
			}
			if eventPeriodEnd.IsZero() || eventTime.After(eventPeriodEnd) {
				eventPeriodEnd = eventTime
			}
			weekKey := analyticsWeekKey(eventTime)
			monthKey := eventTime.Format("2006-01")
			yearKey := eventTime.Format("2006")
			if month := monthly[monthKey]; month != nil {
				month.VolunteerEvents++
			}
			for _, earner := range event.EarnerAddresses {
				address := normalizeAnalyticsAddress(earner)
				if address == "" {
					continue
				}
				analyticsAddNestedSet(weeklyEarners, weekKey, address)
				analyticsAddNestedSet(monthlyEarners, monthKey, address)
				analyticsAddNestedSet(yearlyEarners, yearKey, address)
				if month := monthly[monthKey]; month != nil {
					month.UniqueEarners[address] = struct{}{}
				}
			}
		}
		totalEventCodes += event.CodeCount
		totalRedeemedCodes += event.RedeemedCount
	}

	repeatVolunteerEarners := 0
	participationCount := 0
	for _, count := range volunteerParticipation {
		participationCount += count
		if count > 1 {
			repeatVolunteerEarners++
		}
	}

	monthlyRows := make([]*structs.AdminAnalyticsMonthlyPoint, 0, len(monthlyOrder))
	for _, key := range monthlyOrder {
		bucket := monthly[key]
		if bucket == nil {
			continue
		}
		monthlyRows = append(monthlyRows, &structs.AdminAnalyticsMonthlyPoint{
			Key:                           bucket.Key,
			Label:                         bucket.Label,
			ActiveUsers:                   len(bucket.ActiveUsers),
			TransactionVolumeWei:          analyticsBigIntString(bucket.TransactionVolume),
			DistributedWei:                analyticsBigIntString(bucket.Distributed),
			MerchantSpendWei:              analyticsBigIntString(bucket.MerchantSpend),
			MerchantCustomerWallets:       len(bucket.MerchantCustomers),
			MerchantRepeatCustomerWallets: analyticsRepeatCount(bucket.MerchantCustomers),
			VolunteerEvents:               bucket.VolunteerEvents,
			UniqueEarners:                 len(bucket.UniqueEarners),
			AverageProjectCostWei:         analyticsAverage(bucket.ProjectCost, bucket.ProjectCount),
		})
	}

	dailyRows := make([]*structs.AdminAnalyticsDailyPoint, 0, len(dailyOrder))
	for _, key := range dailyOrder {
		bucket := daily[key]
		if bucket == nil {
			continue
		}
		dailyRows = append(dailyRows, &structs.AdminAnalyticsDailyPoint{
			Key:         bucket.Key,
			Label:       bucket.Label,
			ActiveUsers: len(bucket.ActiveUsers),
		})
	}

	response := structs.AdminAnalyticsResponse{
		GeneratedAt:             now.Unix(),
		ConfiguredPaidAddresses: paidAddresses,
		Summary: structs.AdminAnalyticsSummary{
			TotalActiveUsers:                 totalActiveUsers,
			DailyActiveUsers:                 len(dailyActiveNow),
			MonthlyActiveUsers:               len(monthlyActiveNow),
			PreviousMonthlyActiveUsers:       len(monthlyActivePrevious),
			MonthlyActiveUserChangePercent:   analyticsChangePercent(len(monthlyActiveNow), len(monthlyActivePrevious)),
			MonthlyTransactionVolumeWei:      analyticsBigIntString(monthly[monthStart.Format("2006-01")].TransactionVolume),
			TotalDistributedWei:              analyticsBigIntString(totalDistributed),
			DistributedSpentAtMerchantsWei:   analyticsBigIntString(distributedSpentAtMerchants),
			TokenRedeemedWei:                 analyticsBigIntString(tokenRedeemed),
			TokenUnusedWei:                   analyticsBigIntString(tokenUnused),
			TokenRedeemedPercent:             analyticsPercentage(tokenRedeemed, totalDistributed),
			AverageCommunityProjectCostWei:   analyticsAverage(totalProjectCost, projectCount),
			MerchantCustomerWallets:          len(merchantCustomers),
			MerchantRepeatCustomerWallets:    analyticsRepeatCount(merchantCustomers),
			MerchantRepeatCustomerPercent:    analyticsRatioPercent(analyticsRepeatCount(merchantCustomers), len(merchantCustomers)),
			AverageSecondsToUseDistribution:  analyticsAverageInt64(latencyTotal, latencyCount),
			AverageVolunteerEventsPerWeek:    analyticsAverageEvents(len(volunteerEvents), analyticsPeriodCount(eventPeriodStart, eventPeriodEnd, "week")),
			AverageVolunteerEventsPerMonth:   analyticsAverageEvents(len(volunteerEvents), analyticsPeriodCount(eventPeriodStart, eventPeriodEnd, "month")),
			AverageVolunteerEventsPerYear:    analyticsAverageEvents(len(volunteerEvents), analyticsPeriodCount(eventPeriodStart, eventPeriodEnd, "year")),
			AverageUniqueEarnersPerWeek:      analyticsAverageNestedSet(weeklyEarners),
			AverageUniqueEarnersPerMonth:     analyticsAverageNestedSet(monthlyEarners),
			AverageUniqueEarnersPerYear:      analyticsAverageNestedSet(yearlyEarners),
			VolunteerParticipationCount:      participationCount,
			VolunteerUniqueEarners:           len(volunteerParticipation),
			VolunteerRepeatEarners:           repeatVolunteerEarners,
			VolunteerRepeatParticipationRate: analyticsRatioPercent(repeatVolunteerEarners, len(volunteerParticipation)),
			VolunteerEvents:                  len(volunteerEvents),
			EventCodeRedemptionPercent:       analyticsRatioPercent(totalRedeemedCodes, totalEventCodes),
		},
		Monthly:           monthlyRows,
		Daily:             dailyRows,
		MetricDefinitions: adminAnalyticsDefinitions(),
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func normalizeAnalyticsAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

func analyticsSetFromList(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := normalizeAnalyticsAddress(value)
		if normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set
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

func analyticsAverage(total *big.Int, count int) string {
	if total == nil || total.Sign() <= 0 || count <= 0 {
		return "0"
	}
	return new(big.Int).Div(new(big.Int).Set(total), big.NewInt(int64(count))).String()
}

func analyticsPercentage(part *big.Int, total *big.Int) float64 {
	if part == nil || total == nil || total.Sign() <= 0 {
		return 0
	}
	return analyticsRatioPercentBig(part, total)
}

func analyticsRatioPercentBig(part *big.Int, total *big.Int) float64 {
	if total == nil || total.Sign() <= 0 {
		return 0
	}
	partFloat, _ := new(big.Float).SetInt(part).Float64()
	totalFloat, _ := new(big.Float).SetInt(total).Float64()
	if totalFloat == 0 {
		return 0
	}
	return math.Round((partFloat/totalFloat)*1000) / 10
}

func analyticsRatioPercent(part int, total int) float64 {
	if part <= 0 || total <= 0 {
		return 0
	}
	return math.Round((float64(part)/float64(total))*1000) / 10
}

func analyticsChangePercent(current int, previous int) float64 {
	if previous <= 0 {
		if current > 0 {
			return 100
		}
		return 0
	}
	return math.Round(((float64(current-previous)/float64(previous))*100)*10) / 10
}

func analyticsAverageInt64(total int64, count int64) int64 {
	if total <= 0 || count <= 0 {
		return 0
	}
	return total / count
}

func minAnalyticsBigInt(a *big.Int, b *big.Int) *big.Int {
	if a == nil {
		return big.NewInt(0)
	}
	if b == nil {
		return new(big.Int).Set(a)
	}
	if a.Cmp(b) <= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}

func addActiveAnalyticsUser(
	ownerByAddress map[string]adminAnalyticsOwnerMeta,
	address string,
	txTime time.Time,
	month *adminAnalyticsMonthBucket,
	daily map[string]*adminAnalyticsDayBucket,
	monthlyActiveNow map[string]struct{},
	monthlyActivePrevious map[string]struct{},
	dailyActiveNow map[string]struct{},
	monthStart time.Time,
	dayStart time.Time,
) {
	meta, ok := ownerByAddress[address]
	if !ok || meta.IsAdmin || meta.UserID == "" {
		return
	}
	if month != nil {
		month.ActiveUsers[meta.UserID] = struct{}{}
	}
	if day := daily[txTime.Format("2006-01-02")]; day != nil {
		day.ActiveUsers[meta.UserID] = struct{}{}
	}
	if !txTime.Before(monthStart) {
		monthlyActiveNow[meta.UserID] = struct{}{}
	}
	if !txTime.Before(monthStart.AddDate(0, -1, 0)) && txTime.Before(monthStart) {
		monthlyActivePrevious[meta.UserID] = struct{}{}
	}
	if !txTime.Before(dayStart) {
		dailyActiveNow[meta.UserID] = struct{}{}
	}
}

func analyticsRepeatCount(counts map[string]int) int {
	repeat := 0
	for _, count := range counts {
		if count > 1 {
			repeat++
		}
	}
	return repeat
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

func analyticsWeekKey(value time.Time) string {
	year, week := value.ISOWeek()
	return strconv.Itoa(year) + "-W" + fmtAnalyticsTwoDigit(week)
}

func fmtAnalyticsTwoDigit(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func analyticsAddNestedSet(target map[string]map[string]struct{}, key string, value string) {
	if key == "" || value == "" {
		return
	}
	set := target[key]
	if set == nil {
		set = make(map[string]struct{})
		target[key] = set
	}
	set[value] = struct{}{}
}

func analyticsAverageNestedSet(target map[string]map[string]struct{}) float64 {
	if len(target) == 0 {
		return 0
	}
	total := 0
	for _, set := range target {
		total += len(set)
	}
	return math.Round((float64(total)/float64(len(target)))*10) / 10
}

func analyticsAverageEvents(count int, periods int) float64 {
	if count <= 0 || periods <= 0 {
		return 0
	}
	return math.Round((float64(count)/float64(periods))*10) / 10
}

func analyticsPeriodCount(start time.Time, end time.Time, unit string) int {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	switch unit {
	case "week":
		days := end.Sub(start).Hours() / 24
		return int(math.Floor(days/7)) + 1
	case "month":
		return (end.Year()-start.Year())*12 + int(end.Month()-start.Month()) + 1
	case "year":
		return end.Year() - start.Year() + 1
	default:
		return 0
	}
}

func adminAnalyticsDefinitions() []structs.AdminAnalyticsDefinition {
	return []structs.AdminAnalyticsDefinition{
		{Key: "active_users", Label: "Active users", Definition: "Registered non-admin users with at least one indexed SFLuv transfer in the daily or monthly window."},
		{Key: "transaction_volume", Label: "Monthly transaction volume", Definition: "Gross SFLuv transfer volume from the indexed ERC20 transfer table for the month."},
		{Key: "redeemed_vs_unused", Label: "Redeemed vs unused", Definition: "Total distributed SFLuv from configured paid admin/faucet addresses, minus current positive registered non-admin wallet balances, capped at distributed amount."},
		{Key: "project_cost", Label: "Community project cost", Definition: "Average bounty cost of completed or paid-out workflows using workflow total bounty, falling back to summed step bounties plus manager bounty."},
		{Key: "merchant_traffic", Label: "Merchant traffic and repeat business", Definition: "Wallet-based proxy: unique non-admin customer wallets sending SFLuv to approved merchant wallets, plus wallets with more than one merchant payment."},
		{Key: "distributed_spent", Label: "Distributed spent at merchants", Definition: "SFLuv sent to approved merchant wallets by non-admin wallets, excluding self-payments and direct paid-admin distributions."},
		{Key: "time_to_use", Label: "Time to use after distribution", Definition: "Average time between a paid-admin/faucet distribution and the recipient wallet's next outgoing SFLuv transfer."},
		{Key: "volunteer_events", Label: "Volunteer events and earners", Definition: "Bot event and redemption-code data, grouped by event start time when present, otherwise expiration time."},
	}
}
