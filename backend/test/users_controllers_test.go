package test

import (
	"context"
	"testing"
	"time"

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

	users, err := AppDb.GetUsers(ctx, 0, 2)
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
