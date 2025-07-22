package test

import (
	"fmt"
	"testing"
)

func GroupUsersControllers(t *testing.T) {
	t.Run("add user controller", UnitAddUserController)
	t.Run("update user info controller", UnitUpdateUserInfoController)
	t.Run("update user role controller", UnitUpdateUserRoleController)
	t.Run("get users paginated controller", UnitGetUsersController)
	t.Run("get user by id controller", UnitGetUserById)
}

func UnitAddUserController(t *testing.T) {
	err := AppDb.AddUser(TEST_USER_1.Id)
	if err != nil {
		t.Fatalf(err.Error())
	}
}

func UnitUpdateUserInfoController(t *testing.T) {
	err := AppDb.UpdateUserInfo(&TEST_USER_1)
	if err != nil {
		t.Fatalf(err.Error())
	}
}

func UnitUpdateUserRoleController(t *testing.T) {
	err := AppDb.UpdateUserRole(TEST_USER_1.Id, "admin", true)
	if err != nil {
		t.Fatalf(err.Error())
	}
}

func UnitGetUserById(t *testing.T) {
	user, err := AppDb.GetUserById(TEST_USER_1.Id)
	if err != nil {
		t.Fatalf(err.Error())
	}

	if user.Id != TEST_USER_1.Id {
		t.Fatalf("ids do not match")
	}
	if *user.Name != *TEST_USER_1.Name {
		t.Fatalf("names do not match")
	}
}

func UnitGetUsersController(t *testing.T) {
	err := AppDb.AddUser(TEST_USER_2.Id)
	if err != nil {
		t.Fatalf(err.Error())
	}

	users, err := AppDb.GetUsers(0, 2)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if len(users) != 2 {
		fmt.Println(*users[0])
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
	}
}
