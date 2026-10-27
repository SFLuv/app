package handlers

import (
	"encoding/json"
	"net/http"
)

// GetFundingBalances returns every protocol account's balance for the admin
// panel's Balances tab.
//
// Reads are live rather than cached: the page exists to answer "is anything
// about to break", and a cached answer to that is worse than a slow one. It is
// admin-only and hand-opened, so the request volume is a person clicking.
func (s *BotService) GetFundingBalances(w http.ResponseWriter, r *http.Request) {
	if s.funding == nil {
		http.Error(w, "funding is not configured on this deployment", http.StatusServiceUnavailable)
		return
	}
	snapshot := s.funding.Balances(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snapshot); err != nil && s.app != nil {
		s.app.logger.Logf("error encoding funding balances: %s", err)
	}
}

// RunFundingSweep triggers a top-up pass by hand, for an admin who does not
// want to wait for the next scheduled one.
func (s *BotService) RunFundingSweep(w http.ResponseWriter, r *http.Request) {
	if s.funding == nil {
		http.Error(w, "funding is not configured on this deployment", http.StatusServiceUnavailable)
		return
	}
	result := s.funding.RunOnce(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"funded":  result.Funded,
		"skipped": result.Skipped,
		"errors":  result.Errors,
		"alerted": result.Alerted,
	}); err != nil && s.app != nil {
		s.app.logger.Logf("error encoding funding sweep result: %s", err)
	}
}
