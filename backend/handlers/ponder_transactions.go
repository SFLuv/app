package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/utils"
)

func (p *PonderService) GetTransactionHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	address := query.Get("address")
	page := query.Get("page")
	count := query.Get("count")
	descending := false
	if query.Get("desc") == "true" {
		descending = true
	}

	if page == "" {
		page = "0"
	}

	if count == "" {
		count = "10"
	}

	if address == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	iPage, err := strconv.Atoi(page)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	iCount, err := strconv.Atoi(count)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	txs, err := p.db.GetTransactionsPaginated(r.Context(), address, iPage, iCount, descending)
	if err != nil {
		p.logger.Logf("error getting paginated transactions: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	userDid := utils.GetDid(r)
	if txs != nil && len(txs.Transactions) > 0 && userDid != nil {
		ownedAddresses, addressErr := p.appDB.GetOwnedWalletAddressSet(r.Context(), *userDid)
		if addressErr != nil {
			p.logger.Logf("error loading wallet addresses for memo auth user %s: %s", *userDid, addressErr)
		} else if len(ownedAddresses) > 0 {
			authorizedHashes := make([]string, 0, len(txs.Transactions))
			for _, tx := range txs.Transactions {
				if tx == nil {
					continue
				}

				from := strings.ToLower(strings.TrimSpace(tx.From))
				to := strings.ToLower(strings.TrimSpace(tx.To))
				_, fromOwned := ownedAddresses[from]
				_, toOwned := ownedAddresses[to]
				if !fromOwned && !toOwned {
					continue
				}

				normalizedHash := strings.ToLower(strings.TrimSpace(tx.Hash))
				if normalizedHash == "" {
					continue
				}
				authorizedHashes = append(authorizedHashes, normalizedHash)
			}

			memosByHash, memoErr := p.appDB.GetTransactionMemosByHashes(r.Context(), authorizedHashes)
			if memoErr != nil {
				p.logger.Logf("error loading authorized memos for user %s address %s: %s", *userDid, address, memoErr)
			} else {
				for _, tx := range txs.Transactions {
					if tx == nil {
						continue
					}
					memo, ok := memosByHash[strings.ToLower(strings.TrimSpace(tx.Hash))]
					if ok && strings.TrimSpace(memo) != "" {
						tx.Memo = memo
					}
				}
			}
		}
	}

	bytes, err := json.Marshal(txs)
	if err != nil {
		p.logger.Logf("error marshalling transactions page into response: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

type upsertTransactionMemoRequest struct {
	TxHash string `json:"tx_hash"`
	Memo   string `json:"memo"`
}

func (p *PonderService) UpsertTransactionMemo(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var req upsertTransactionMemoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	txHash := strings.TrimSpace(req.TxHash)
	memo := strings.TrimSpace(req.Memo)
	if txHash == "" || memo == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(memo) > 1000 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	txParties, txErr := p.db.GetTransactionPartiesByHash(r.Context(), txHash)
	if txErr != nil {
		p.logger.Logf("error looking up transaction %s for memo authorization: %s", txHash, txErr)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Optimistic memo writes: allow storing memo by tx hash even if the tx has
	// not been indexed/confirmed yet. When tx parties are available, enforce
	// sender/recipient ownership authorization.
	if txParties != nil {
		authorized, authErr := p.appDB.UserOwnsAnyWalletAddress(r.Context(), *userDid, []string{txParties.From, txParties.To})
		if authErr != nil {
			p.logger.Logf("error checking memo authorization for tx %s user %s: %s", txHash, *userDid, authErr)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !authorized {
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	err := p.appDB.UpsertTransactionMemo(r.Context(), txHash, memo, *userDid)
	if err != nil {
		p.logger.Logf("error upserting transaction memo for tx %s user %s: %s", txHash, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := map[string]string{
		"tx_hash": strings.ToLower(txHash),
		"memo":    memo,
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (p *PonderService) GetBalanceAtTimestamp(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	address := query.Get("address")
	timestamp := query.Get("timestamp")
	if address == "" || timestamp == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	parsedTimestamp, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	balance, err := p.db.GetBalanceAtTimestamp(r.Context(), address, parsedTimestamp)
	if err != nil {
		p.logger.Logf("error getting balance at timestamp: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"address":   address,
		"timestamp": parsedTimestamp,
		"balance":   balance,
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		p.logger.Logf("error marshalling balance response: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}
