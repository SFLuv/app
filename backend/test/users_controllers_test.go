package test

import (
	"context"
	"testing"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

func GroupUsersControllers(t *testing.T) {
	t.Run("add user controller", ModuleAddUserController)
	t.Run("update user info controller", ModuleUpdateUserInfoController)
	t.Run("update user role controller", ModuleUpdateUserRoleController)
	t.Run("get users paginated controller", ModuleGetUsersController)
	t.Run("get user by id controller", ModuleGetUserById)
	t.Run("account deletion controller", ModuleAccountDeletionController)
}

func ModuleAddUserController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := AppDb.AddUser(ctx, TEST_USER_1.Id)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = AppDb.AddUser(ctx, TEST_USER_2.Id)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleUpdateUserInfoController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := AppDb.UpdateUserInfo(ctx, &TEST_USER_1)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = AppDb.UpdateUserInfo(ctx, &TEST_USER_2)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleUpdateUserRoleController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := AppDb.UpdateUserRole(ctx, TEST_USER_1.Id, "admin", true)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleGetUserById(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	user, err := AppDb.GetUserById(ctx, TEST_USER_1.Id)
	if err != nil {
		t.Fatal(err.Error())
	}

	if user.Id != TEST_USER_1.Id {
		t.Fatalf("ids do not match")
	}
	if *user.Name != *TEST_USER_1.Name {
		t.Fatalf("names do not match")
	}
}

func ModuleGetUsersController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	users, err := AppDb.GetUsers(ctx, 0, 2, "", nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(users) != 2 {
		t.Fatalf("incorrect users array length %d", len(users))
	}

	for n, user := range users {
		if user.Id != TEST_USERS[n].Id {
			t.Fatalf("ids do not match for user %d", n)
		}
		if user.Name != nil &&
			TEST_USERS[n].Name != nil &&
			*user.Name != *TEST_USERS[n].Name {
			t.Fatalf("names do not match for user %d", n)
		}
		if user.Email == nil {
			t.Fatalf("expected to find user %d email", n+1)
		}
		if *user.Email != *TEST_USERS[n].Email {
			t.Fatalf("email %s does not match expected %s", *user.Email, *TEST_USERS[n].Email)
		}
	}

	older := time.Now().UTC().Add(-2 * time.Hour)
	if err := AppDb.RecordClientVersionObservation(ctx, structs.ClientVersionObservation{
		UserId:         TEST_USER_1.Id,
		ClientKey:      "test:user-1:platform:mobile",
		Platform:       "mobile",
		Version:        "1.0.0",
		Build:          "1",
		BuildNumber:    1,
		LegacyInferred: true,
		SeenAt:         older,
	}); err != nil {
		t.Fatalf("error recording older user 1 client version: %s", err)
	}
	if err := AppDb.RecordClientVersionObservation(ctx, structs.ClientVersionObservation{
		UserId:      TEST_USER_1.Id,
		ClientKey:   "test:user-1:platform:ios",
		Platform:    "ios",
		Version:     "2.0.0",
		Build:       "7",
		BuildNumber: 7,
		SeenAt:      older.Add(time.Hour),
	}); err != nil {
		t.Fatalf("error recording latest user 1 client version: %s", err)
	}
	if err := AppDb.RecordClientVersionObservation(ctx, structs.ClientVersionObservation{
		UserId:         TEST_USER_2.Id,
		ClientKey:      "test:user-2:platform:mobile",
		Platform:       "mobile",
		Version:        "1.0.0",
		Build:          "1",
		BuildNumber:    1,
		LegacyInferred: true,
		SeenAt:         older.Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("error recording user 2 client version: %s", err)
	}

	oldVersionUsers, err := AppDb.GetUsers(ctx, 0, 10, "", []string{"1.0.0 (1)"})
	if err != nil {
		t.Fatalf("error filtering users by old client version: %s", err)
	}
	if len(oldVersionUsers) != 1 || oldVersionUsers[0].Id != TEST_USER_2.Id {
		t.Fatalf("expected old-version filter to include only %s, got %#v", TEST_USER_2.Id, oldVersionUsers)
	}

	newVersionUsers, err := AppDb.GetUsers(ctx, 0, 10, "", []string{"2.0.0 (7)"})
	if err != nil {
		t.Fatalf("error filtering users by new client version: %s", err)
	}
	if len(newVersionUsers) != 1 || newVersionUsers[0].Id != TEST_USER_1.Id {
		t.Fatalf("expected new-version filter to include only %s, got %#v", TEST_USER_1.Id, newVersionUsers)
	}

	counts, err := AppDb.GetClientVersionUserCounts(ctx)
	if err != nil {
		t.Fatalf("error getting client version user counts: %s", err)
	}
	countByLabel := map[string]int{}
	for _, count := range counts {
		countByLabel[count.VersionLabel] = count.UserCount
	}
	if countByLabel["1.0.0 (1)"] != 1 {
		t.Fatalf("expected one current old-version user, got %d", countByLabel["1.0.0 (1)"])
	}
	if countByLabel["2.0.0 (7)"] != 1 {
		t.Fatalf("expected one current new-version user, got %d", countByLabel["2.0.0 (7)"])
	}
}

func ModuleAccountDeletionController(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	userID := "test-delete-controller"
	if err := AppDb.AddUser(ctx, userID); err != nil {
		t.Fatalf("error creating deletion test user: %s", err)
	}

	preview, err := AppDb.GetAccountDeletionPreview(ctx, userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("error getting deletion preview: %s", err)
	}
	if preview.Status != "active" {
		t.Fatalf("expected active preview status, got %s", preview.Status)
	}

	status, err := AppDb.ScheduleAccountDeletion(ctx, userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("error scheduling account deletion: %s", err)
	}
	if status.Status != "scheduled_for_deletion" {
		t.Fatalf("expected scheduled status, got %s", status.Status)
	}
	if !status.CanCancel {
		t.Fatal("expected scheduled deletion to be cancelable")
	}

	if _, err := AppDb.GetUserById(ctx, userID); err != pgx.ErrNoRows {
		t.Fatalf("expected active lookup to hide deleted user, got err=%v", err)
	}

	status, err = AppDb.CancelAccountDeletion(ctx, userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("error canceling account deletion: %s", err)
	}
	if status.Status != "active" {
		t.Fatalf("expected active status after cancel, got %s", status.Status)
	}
}
