package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

// Admin cash-out destination. Kept separate from the merchant per-location
// liquidation address (app_location.go) on purpose: admins have no locations,
// so their address lives on the user row and has its own routes.

func (a *AppService) GetAdminLiquidationAddress(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	liquidationAddress, err := a.db.GetAdminLiquidationAddress(r.Context(), *userDid)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error reading admin liquidation address for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(structs.AdminLiquidationAddressResponse{
		LiquidationAddress: liquidationAddress,
	})
}

func (a *AppService) UpdateAdminLiquidationAddress(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading admin liquidation address body for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var request structs.AdminLiquidationAddressUpdateRequest
	if err := json.Unmarshal(body, &request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	normalizedAddress, err := a.db.UpdateAdminLiquidationAddress(r.Context(), *userDid, request.LiquidationAddress)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err.Error() == "liquidation address must be a valid ethereum address" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		a.logger.Logf("error updating admin liquidation address for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(structs.AdminLiquidationAddressResponse{
		LiquidationAddress: normalizedAddress,
	})
}
