package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

func parsePositiveInt64(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func transactionMemoKey(chainID int64, txHash string) string {
	return strconv.FormatInt(chainID, 10) + ":" + strings.ToLower(strings.TrimSpace(txHash))
}

func (p *PonderService) transactionMemosByChainHash(ctx context.Context, authorizedHashes []string, txs []*structs.PonderTransaction) (map[string]string, error) {
	authorized := make(map[string]struct{}, len(authorizedHashes))
	for _, hash := range authorizedHashes {
		hash = strings.ToLower(strings.TrimSpace(hash))
		if hash != "" {
			authorized[hash] = struct{}{}
		}
	}
	if len(authorized) == 0 {
		return map[string]string{}, nil
	}

	hashesByChain := make(map[int64][]string)
	seenByChainHash := make(map[string]struct{})
	for _, tx := range txs {
		if tx == nil || tx.ChainID <= 0 {
			continue
		}
		hash := strings.ToLower(strings.TrimSpace(tx.Hash))
		if hash == "" {
			continue
		}
		if _, ok := authorized[hash]; !ok {
			continue
		}
		key := transactionMemoKey(tx.ChainID, hash)
		if _, ok := seenByChainHash[key]; ok {
			continue
		}
		seenByChainHash[key] = struct{}{}
		hashesByChain[tx.ChainID] = append(hashesByChain[tx.ChainID], hash)
	}
	if len(hashesByChain) == 0 {
		return map[string]string{}, nil
	}

	memosByTx := make(map[string]string)
	for chainID, hashes := range hashesByChain {
		memosByHash, err := p.appDB.GetTransactionMemosByHashes(ctx, hashes, chainID)
		if err != nil {
			return nil, err
		}
		for hash, memo := range memosByHash {
			memosByTx[transactionMemoKey(chainID, hash)] = memo
		}
	}

	return memosByTx, nil
}

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

			memosByTx, memoErr := p.transactionMemosByChainHash(r.Context(), authorizedHashes, txs.Transactions)
			if memoErr != nil {
				p.logger.Logf("error loading authorized memos for user %s address %s: %s", *userDid, address, memoErr)
			} else {
				for _, tx := range txs.Transactions {
					if tx == nil {
						continue
					}
					memo, ok := memosByTx[transactionMemoKey(tx.ChainID, tx.Hash)]
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
	TxHash  string `json:"tx_hash"`
	ChainID int64  `json:"chain_id"`
	Memo    string `json:"memo"`
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
	chainID := req.ChainID

	txParties, txErr := p.db.GetTransactionPartiesByHash(r.Context(), txHash, chainID)
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
		if chainID <= 0 {
			chainID = txParties.ChainID
		}
	}
	if chainID <= 0 {
		chainID = p.requestChainID(r)
	}

	err := p.appDB.UpsertTransactionMemo(r.Context(), txHash, chainID, memo, *userDid)
	if err != nil {
		p.logger.Logf("error upserting transaction memo for tx %s user %s: %s", txHash, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"tx_hash":  strings.ToLower(txHash),
		"chain_id": chainID,
		"memo":     memo,
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
	chainID := p.requestChainID(r)

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
		"chain_id":  chainID,
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
