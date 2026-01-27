package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
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

	if address == "" || page == "" || count == "" {
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

	bytes, err := json.Marshal(txs)
	if err != nil {
		p.logger.Logf("error marshalling transactions page into response: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}
