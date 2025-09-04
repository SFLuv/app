package test

import (
	"context"
	"testing"
)

func GroupWalletsControllers(t *testing.T) {
	t.Run("add wallet controller", ModuleAddWalletController)
	t.Run("update wallet controller", ModuleUpdateWalletControler)
	t.Run("get wallets by user controller", ModuleGetWalletsByUserController)
}

func ModuleAddWalletController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	id, err := AppDb.AddWallet(ctx, &TEST_WALLET_1)
	if err != nil {
		t.Fatal(err.Error())
	}

	if id != 1 {
		t.Fatalf("expected id 1 got %d", id)
	}

	_, err = AppDb.AddWallet(ctx, &TEST_WALLET_2)
	if err != nil {
		t.Fatalf("error adding second wallet: %s", err.Error())
	}
}

func ModuleUpdateWalletControler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := AppDb.UpdateWallet(ctx, &TEST_WALLET_1A)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleGetWalletsByUserController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wallets, err := AppDb.GetWalletsByUser(ctx, TEST_WALLET_1.Owner)
	if err != nil {
		t.Fatal(err.Error())
	}

	for n, wallet := range wallets {
		if wallet.Owner != TEST_WALLETS[n].Owner {
			t.Fatalf("ids do not match for wallet %d: got %s expected %s", *wallet.Id, wallet.Owner, TEST_WALLETS[n].Owner)
		}
		if wallet.Name != TEST_WALLETS[n].Name {
			t.Fatalf("names do not match for wallet %d: got %s expected %s", *wallet.Id, wallet.Name, TEST_WALLETS[n].Name)
		}
		if wallet.IsEoa != TEST_WALLETS[n].IsEoa {
			t.Fatalf("eoa type does not match for wallet %d: got %t expected %t", *wallet.Id, wallet.IsEoa, TEST_WALLETS[n].IsEoa)
		}
	}
}
