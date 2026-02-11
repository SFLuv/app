package structs

type Affiliate struct {
	UserId           string  `json:"user_id"`
	Organization     string  `json:"organization"`
	Nickname         *string `json:"nickname"`
	Status           string  `json:"status"`
	AffiliateLogo    *string `json:"affiliate_logo"`
	WeeklyAllocation uint64  `json:"weekly_allocation"`
	WeeklyBalance    uint64  `json:"weekly_balance"`
	OneTimeBalance   uint64  `json:"one_time_balance"`
}

type AffiliateWeeklyConfig struct {
	UserId           string `json:"user_id"`
	WeeklyAllocation uint64 `json:"weekly_allocation"`
}

type BalanceReservation struct {
	WeeklyDeducted  uint64 `json:"weekly_deducted"`
	OneTimeDeducted uint64 `json:"one_time_deducted"`
}

type AffiliateRequest struct {
	Organization string `json:"organization"`
}

type AffiliateLogoRequest struct {
	Logo string `json:"logo"`
}

type AffiliateUpdateRequest struct {
	UserId         string  `json:"user_id"`
	Status         *string `json:"status,omitempty"`
	Nickname       *string `json:"nickname,omitempty"`
	WeeklyBalance  *uint64 `json:"weekly_balance,omitempty"`
	OneTimeBalance *uint64 `json:"one_time_balance,omitempty"`
}

type AffiliateBalance struct {
	Available        uint64 `json:"available"`
	WeeklyAllocation uint64 `json:"weekly_allocation"`
	WeeklyBalance    uint64 `json:"weekly_balance"`
	OneTimeBalance   uint64 `json:"one_time_balance"`
	Reserved         uint64 `json:"reserved"`
}
