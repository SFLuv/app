package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func locationWalletRole(r *http.Request) (string, error) {
	role := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "role")))
	if role == "" {
		role = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	}
	switch role {
	case db.LocationWalletRolePayment, db.LocationWalletRoleTipping:
		return role, nil
	case "":
		return db.LocationWalletRolePayment, nil
	default:
		return "", fmt.Errorf("unknown wallet role %q", role)
	}
}

// GetAssignableWallets lists the merchant's wallets for a location's wallet
// picker, including the ones already spoken for and why.
func (a *AppService) GetAssignableWallets(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	role, err := locationWalletRole(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	wallets, err := a.db.GetAssignableWalletsForLocation(r.Context(), *userDid, locationID, role)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error listing assignable wallets for user %s and location %d: %s", *userDid, locationID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(structs.AssignableWalletsResponse{Wallets: wallets})
}

// ReplaceLocationWallet swaps the wallet filling one role at a location, either
// for one the merchant already owns or for a freshly derived address.
//
// Derivation happens here, before the database transaction opens, so no write
// locks are held while waiting on the chain. If the chain is unreachable the
// swap fails cleanly and the merchant keeps the wallet they had.
func (a *AppService) ReplaceLocationWallet(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	locationID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	role, err := locationWalletRole(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading wallet replacement body for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var request structs.LocationWalletReplaceRequest
	if err := json.Unmarshal(body, &request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	mode := strings.ToLower(strings.TrimSpace(request.Mode))
	var newWallet *db.NewLocationWallet

	switch mode {
	case "new":
		newWallet, err = a.deriveWalletForLocation(r, locationID, role)
		if err != nil {
			a.logger.Logf("error deriving a %s wallet for location %d: %s", role, locationID, err)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Could not reach the chain to create a new wallet. Try again in a moment."))
			return
		}
	case "existing", "":
		if strings.TrimSpace(request.Address) == "" && role == db.LocationWalletRolePayment {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("A location must always have a payment wallet — choose another wallet or create a new one."))
			return
		}
	default:
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Unknown mode. Use \"existing\" or \"new\"."))
		return
	}

	location, err := a.db.ReplaceLocationWallet(r.Context(), *userDid, locationID, role, request.Address, newWallet)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		errMsg := err.Error()
		if containsLocationWalletValidationError(errMsg) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}

		a.logger.Logf("error replacing the %s wallet for user %s and location %d: %s", role, *userDid, locationID, errMsg)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(location)
}

// deriveWalletForLocation works out the next free smart-account index for the
// merchant and asks the account factory what address it will have.
func (a *AppService) deriveWalletForLocation(r *http.Request, locationID uint64, role string) (*db.NewLocationWallet, error) {
	provisioning, err := a.db.GetLocationProvisioningContext(r.Context(), uint(locationID))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(provisioning.OwnerEOA) == "" {
		return nil, fmt.Errorf("this account has no signing wallet to derive a new address from")
	}

	index := provisioning.DerivationStartIndex()
	address, err := a.deriveSmartAccountAddress(r.Context(), provisioning.OwnerEOA, index)
	if err != nil {
		return nil, err
	}

	suffix := "Payments"
	if role == db.LocationWalletRoleTipping {
		suffix = "Tips"
	}

	return &db.NewLocationWallet{
		Address: address,
		Index:   index,
		Name:    a.db.UniqueWalletName(r.Context(), provisioning.OwnerID, provisioning.Street+" - "+suffix),
	}, nil
}
