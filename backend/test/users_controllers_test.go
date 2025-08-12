package test

import (
	"testing"
)

func GroupUsersControllers(t *testing.T) {
	t.Run("add user controller", ModuleAddUserController)
	t.Run("update user info controller", ModuleUpdateUserInfoController)
	t.Run("update user role controller", ModuleUpdateUserRoleController)
	t.Run("get users paginated controller", ModuleGetUsersController)
	t.Run("get user by id controller", ModuleGetUserById)
}

func ModuleAddUserController(t *testing.T) {
	err := AppDb.AddUser(TEST_USER_1.Id)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = AppDb.AddUser(TEST_USER_2.Id)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleUpdateUserInfoController(t *testing.T) {
	err := AppDb.UpdateUserInfo(&TEST_USER_1)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = AppDb.UpdateUserInfo(&TEST_USER_2)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleUpdateUserRoleController(t *testing.T) {
	err := AppDb.UpdateUserRole(TEST_USER_1.Id, "admin", true)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleGetUserById(t *testing.T) {
	user, err := AppDb.GetUserById(TEST_USER_1.Id)
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
	users, err := AppDb.GetUsers(0, 2)
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
