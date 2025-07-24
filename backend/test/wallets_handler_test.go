package test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func GroupWalletsHandlers(t *testing.T) {
	t.Run("add wallet handler", ModuleAddWalletHandler)
	t.Run("get wallets by user handler", ModuleGetWalletsByUserHandler)
}

func ModuleAddWalletHandler(t *testing.T) {
	Spoofer.SetValue("userDid", TEST_USER_1.Id)

	body, err := json.Marshal(TEST_WALLET_1)
	if err != nil {
		t.Fatalf("error marshalling user for request body: %s", err)
	}

	req, err := http.NewRequest(http.MethodPost, TestServer.URL+"/wallets", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}

	res, err := TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending request: %s", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", res.StatusCode)
	}
}

func ModuleGetWalletsByUserHandler(t *testing.T) {
	Spoofer.SetValue("userDid", TEST_USER_1.Id)

	req, err := http.NewRequest(http.MethodGet, TestServer.URL+"/wallets", nil)
	if err != nil {
		t.Fatalf("error creating get request: %s", err)
	}

	res, err := TestServer.Client().Do(req)
	if err != nil {
		t.Fatalf("error sending get request: %s", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("request failed, got response code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("error reading response body %s", err)
	}

	var wallets []*structs.Wallet
	err = json.Unmarshal(body, &wallets)
	if err != nil {
		t.Fatalf("error unmarshalling response body %s", err)
	}

	if len(wallets) != 1 {
		t.Fatalf("expected wallet length 1, got %d", len(wallets))
	}

	wallet := wallets[0]
	if wallet.Owner != TEST_WALLET_1.Owner {
		t.Fatalf("ids do not match for wallet")
	}
	if wallet.Name != TEST_WALLET_1.Name {
		t.Fatalf("names do not match for wallet")
	}
	if wallet.IsEoa != TEST_WALLET_1.IsEoa {
		t.Fatalf("eoa type does not match for wallet")
	}
}
