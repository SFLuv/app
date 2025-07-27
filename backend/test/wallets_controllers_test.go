package test

import "testing"

func GroupWalletsControllers(t *testing.T) {
	t.Run("add wallet controller", ModuleAddWalletController)
	t.Run("get wallets by user controller", ModuleGetWalletsByUserController)
}

func ModuleAddWalletController(t *testing.T) {
	err := AppDb.AddWallet(&TEST_WALLET_1)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleGetWalletsByUserController(t *testing.T) {
	err := AppDb.AddWallet(&TEST_WALLET_2)
	if err != nil {
		t.Fatalf("error adding second wallet")
	}

	wallets, err := AppDb.GetWalletsByUser(TEST_WALLET_1.Owner)
	if err != nil {
		t.Fatal(err.Error())
	}

	for n, wallet := range wallets {
		if wallet.Owner != TEST_WALLETS[n].Owner {
			t.Fatalf("ids do not match for wallet %d", n)
		}
		if wallet.Name != TEST_WALLETS[n].Name {
			t.Fatalf("names do not match for wallet %d", n)
		}
		if wallet.IsEoa != TEST_WALLETS[n].IsEoa {
			t.Fatalf("eoa type does not match for wallet %d", n)
		}
	}
}
