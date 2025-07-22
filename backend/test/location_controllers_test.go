package test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func GroupLocationControllers(t *testing.T) {
	t.Run("add location controller", ModuleAddLocation)
	t.Run("update location controller", ModuleUpdateLocation)
	t.Run("get all locations controller", ModuleGetLocationsController)
	t.Run("get location by location id controller", ModuleGetLocationByIDController)
	t.Run("get location by user id controller", ModuleGetLocationByUserController)
}

func ModuleAddLocationController(t *testing.T) {
	err := AppDb.AddLocation(&TEST_LOCATION_1)
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

	if len(locations) != 2 {
		fmt.Println(*locations[0])
		t.Fatalf("incorrect location array length %d", len(locations))
	}

	for n, location := range locations {
		if location != &TEST_LOCATIONS[n] {
			t.Fatalf("location info does not match for location %d", n)
		}
	}

}

func ModuleGetLocationByUserController(t *testing.T) {
	locations, err := AppDb.GetLocationsByUser(TEST_USER_1.Id)
	if err != nil {
		t.Fatal(err.Error())
	}

	if *locations[0] != TEST_LOCATION_1 {
		t.Fatalf("location does not match test user 1's location ")
	}

	returned_location_value := reflect.ValueOf(*locations[0])
	test_location_value := reflect.ValueOf(TEST_LOCATION_1)
	returned_location_type := reflect.TypeOf(*locations[0])

	for i := range returned_location_value.NumField() {
		if returned_location_value.Field(i) != test_location_value.Field(i) {
			t.Errorf("returned location field %v does not match test location", returned_location_type.Field(i))
		}
	}

}

func ModuleGetLocationByIDController(t *testing.T) {
	location, err := AppDb.GetLocation(1)
	if err != nil {
		t.Fatal(err.Error())
	}

	returned_location_value := reflect.ValueOf(*location)
	test_location_value := reflect.ValueOf(TEST_LOCATION_1)
	returned_location_type := reflect.TypeOf(*location)

	for i := range returned_location_value.NumField() {
		if returned_location_value.Field(i) != test_location_value.Field(i) {
			t.Errorf("returned location field %v does not match test location", returned_location_type.Field(i))
		}
	}
}
