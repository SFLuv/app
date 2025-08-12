package test

import (
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func GroupLocationControllers(t *testing.T) {
	t.Run("add location controller", ModuleAddLocationController)
	t.Run("update location controller", ModuleUpdateLocationController)
	t.Run("get all locations controller", ModuleGetLocationsController)
	t.Run("get location by location id controller", ModuleGetLocationByIDController)
	t.Run("get location by user id controller", ModuleGetLocationsByUserController)
}

func ModuleAddLocationController(t *testing.T) {
	err := AppDb.AddLocation(&TEST_LOCATION_1)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = AppDb.AddLocation(&TEST_LOCATION_2)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleUpdateLocationController(t *testing.T) {
	err := AppDb.UpdateLocation(&TEST_LOCATION_2A)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func ModuleGetLocationsController(t *testing.T) {
	var location_pagination = structs.LocationsPageRequest{
		Page:  0,
		Count: 1000,
	}
	locations, err := AppDb.GetLocations(&location_pagination)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(locations) != len(TEST_LOCATIONS) {
		t.Fatalf("incorrect location array length %d", len(locations))
	}

	for n, location := range locations {
		if location.ID != TEST_LOCATIONS[n].ID {
			t.Fatalf("got incorrect location value %d, expected %d", location.ID, TEST_LOCATIONS[n].ID)
		}
		if location.GoogleID != TEST_LOCATIONS[n].GoogleID {
			t.Fatalf("got incorrect location value %d, expected %d", location.ID, locations[n].ID)
		}
		if location.Name != TEST_LOCATIONS[n].Name {
			t.Fatalf("got incorrect location name value %s, expected %s", location.Name, TEST_LOCATIONS[n].Name)
		}
	}
}

func ModuleGetLocationsByUserController(t *testing.T) {
	locations, err := AppDb.GetLocationsByUser(TEST_USER_1.Id)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(locations) != 1 {
		t.Fatalf("got incorrect amount of locations in response %d, expected 1", len(locations))
	}

	location := *locations[0]
	if location.ID != 1 {
		t.Fatalf("got incorrect location value %d, expected 1", location.ID)
	}
	if location.GoogleID != TEST_LOCATION_1.GoogleID {
		t.Fatalf("got incorrect location google id value %s, expected %s", location.GoogleID, TEST_LOCATION_1.GoogleID)
	}
	if location.Name != TEST_LOCATION_1.Name {
		t.Fatalf("got incorrect location name value %s, expected %s", location.Name, TEST_LOCATION_1.Name)
	}

}

func ModuleGetLocationByIDController(t *testing.T) {
	location, err := AppDb.GetLocation(1)
	if err != nil {
		t.Fatal(err.Error())
	}

	if location.ID != 1 {
		t.Fatalf("got incorrect location value %d, expected 1", location.ID)
	}
	if location.GoogleID != TEST_LOCATION_1.GoogleID {
		t.Fatalf("got incorrect location google id value %s, expected %s", location.GoogleID, TEST_LOCATION_1.GoogleID)
	}
	if location.Name != TEST_LOCATION_1.Name {
		t.Fatalf("got incorrect location name value %s, expected %s", location.Name, TEST_LOCATION_1.Name)
	}
}
