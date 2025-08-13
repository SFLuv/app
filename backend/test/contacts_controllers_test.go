package test

import (
	"fmt"
	"testing"
)

func GroupContactsControllers(t *testing.T) {
	t.Run("add contact controller", ModuleAddContactController)
	t.Run("update contact controller", ModuleUpdateContactController)
	t.Run("get contacts controller", ModuleGetContactsController)
	t.Run("delete contract controller", ModuleDeleteContactController)
}

func ModuleAddContactController(t *testing.T) {
	id, err := AppDb.AddContact(&TEST_CONTACT_1, TEST_CONTACT_1.Owner)
	if err != nil {
		t.Fatal(err.Error())
	}

	id2, err := AppDb.AddContact(&TEST_CONTACT_2, TEST_CONTACT_1.Owner)
	if err != nil {
		t.Fatal(err.Error())
	}

	fmt.Println(id, id2)
}

func ModuleUpdateContactController(t *testing.T) {
	err := AppDb.UpdateContact(&TEST_CONTACT_2A, TEST_CONTACT_1.Owner)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleGetContactsController(t *testing.T) {
	cs, err := AppDb.GetContacts(TEST_CONTACT_1.Owner)
	if err != nil {
		t.Fatal(err.Error())
	}

	if cs[0].Name != TEST_CONTACT_1.Name {
		t.Fatalf("got incorrect name for contact 1: %s, expected %s", cs[0].Name, TEST_CONTACT_1.Name)
	}
	if cs[1].Address != TEST_CONTACT_2A.Address {
		t.Fatalf("got incorrect address for contact 2: %s, expected %s", cs[1].Address, TEST_CONTACT_2A.Address)
	}
}

func ModuleDeleteContactController(t *testing.T) {
	err := AppDb.DeleteContact(TEST_CONTACT_1.Owner, TEST_CONTACT_1.Id)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = AppDb.DeleteContact(TEST_CONTACT_1.Owner, TEST_CONTACT_1.Id)
	if err != nil {
		t.Fatal(err.Error())
	}
}
