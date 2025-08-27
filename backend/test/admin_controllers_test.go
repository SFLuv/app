package test

import (
	"context"
	"testing"
)

func GroupAdminControllers(t *testing.T) {
	t.Run("update location approval controller", ModuleUpdateLocationApprovalController)
}

func ModuleUpdateLocationApprovalController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := AppDb.UpdateLocationApproval(ctx, TEST_LOCATION_1.ID, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	loc, err := AppDb.GetLocation(ctx, uint64(TEST_LOCATION_1.ID))
	if err != nil {
		t.Fatal(err.Error())
	}

	if !loc.Approval {
		t.Fatal("expected location to be approved")
	}

	user, err := AppDb.GetUserById(ctx, TEST_LOCATION_1.OwnerID)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !user.IsMerchant {
		t.Fatal("expected user to be merchant")
	}

	loc2 := TEST_LOCATION_2
	loc2.OwnerID = TEST_LOCATION_1.OwnerID
	loc2.GoogleID = "test_loc_2"

	err = AppDb.AddLocation(ctx, &loc2)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = AppDb.UpdateLocationApproval(ctx, 3, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !user.IsMerchant {
		t.Fatal("expected user to be merchant after second approval")
	}

	err = AppDb.UpdateLocationApproval(ctx, 3, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !user.IsMerchant {
		t.Fatal("expected user to be merchant after second approval removed")
	}

	err = AppDb.UpdateLocationApproval(ctx, TEST_LOCATION_1.ID, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !user.IsMerchant {
		t.Fatal("expected user not to be a merchant after all locations removed")
	}
}
