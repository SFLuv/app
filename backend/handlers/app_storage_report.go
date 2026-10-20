package handlers

import (
	"net/http"

	"github.com/SFLuv/app/backend/db"
)

// AdminStorageReport sizes the database and every image column in it, with
// bytes added per month. Read-only; it sizes the move of photos to object
// storage.
func (a *AppService) AdminStorageReport(w http.ResponseWriter, r *http.Request) {
	var botDB *db.BotDB
	if a.bot != nil {
		botDB = a.bot.db
	}
	report, err := a.db.StorageReport(r.Context(), botDB)
	if err != nil {
		a.logger.Logf("error building storage report: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
