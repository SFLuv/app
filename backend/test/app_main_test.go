package test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/router"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils/middleware"
	"github.com/go-chi/chi/v5"
)

var Spoofer *ContextSpoofer

type ContextSpoofer struct {
	key   any
	value any
}

func NewContextSpoofer(key any, value any) *ContextSpoofer {
	return &ContextSpoofer{
		key:   key,
		value: value,
	}
}

func (c *ContextSpoofer) SetValue(key any, value any) {
	c.key = key
	c.value = value
}

func (c *ContextSpoofer) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), c.key, c.value)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

var t1e = "test1@test.com"
var t1p = "test1phone"
var t1n = "test1name"
var testTrue = true
var testFalse = false
var TEST_USER_1 = structs.User{
	Id:          "test1",
	Exists:      true,
	IsAdmin:     false,
	IsMerchant:  false,
	IsOrganizer: false,
	IsImprover:  false,
	Email:       &t1e,
	Phone:       &t1p,
	Name:        &t1n,
}

var t2e = "test2@test.com"
var t2p = "test2phone"
var t2n = "test2name"
var TEST_USER_2 = structs.User{
	Id:          "test2",
	Exists:      true,
	IsAdmin:     false,
	IsMerchant:  false,
	IsOrganizer: false,
	IsImprover:  false,
	Email:       &t2e,
	Phone:       &t2p,
	Name:        &t2n,
}

var TEST_USERS = []structs.User{TEST_USER_2, TEST_USER_1}

var TEST_WALLET_1 = structs.Wallet{
	Id:           nil,
	Owner:        TEST_USER_1.Id,
	Name:         "test_wallet",
	IsEoa:        true,
	EoaAddress:   "0x",
	SmartAddress: nil,
	SmartIndex:   nil,
}

var t1Aid = 1
var TEST_WALLET_1A = structs.Wallet{
	Id:           &t1Aid,
	Owner:        TEST_USER_1.Id,
	Name:         "test_wallet_update",
	IsEoa:        true,
	EoaAddress:   "0x",
	SmartAddress: nil,
	SmartIndex:   nil,
}

var t2a = "0x"
var t2i = 0
var TEST_WALLET_2 = structs.Wallet{
	Id:           nil,
	Owner:        TEST_USER_1.Id,
	Name:         "test_smart_wallet",
	IsEoa:        false,
	EoaAddress:   "0x",
	SmartAddress: &t2a,
	SmartIndex:   &t2i,
}

var TEST_WALLETS = []structs.Wallet{TEST_WALLET_1A, TEST_WALLET_2}

var TEST_LOCATION_1 = structs.Location{
	ID:          1,
	GoogleID:    "ChIJQWq7yBaBhYARAW1YO8VAzB4",
	OwnerID:     TEST_USER_1.Id,
	Name:        "Azalina's",
	Description: "cacsac",
	Type:        "Restaurant",
	Approval:    &testTrue,
	Street:      "499 Ellis Street",
	City:        "San Francisco",
	State:       "California",
	ZIP:         "94102",
	Lat:         37.7845922,
	Lng:         -122.4143191,
	Phone:       "4156038101",
	Email:       "sanchez@oleary.com",
	AdminPhone:  "4156038101",
	AdminEmail:  "sanchez@oleary.com",
	Website:     "https://www.azalinas.com/",
	ImageURL:    "https://images.getbento.com/accounts/f8194d40a15b976a1bce5338d622a254/media/images/99097CKmJL5kTBytRFQHlzNzO_533593_567145296630856_962075461_n.png?w=1000&fit=max&auto=compress,format&h=1000",
	Rating:      4.5,
	MapsPage:    "https://maps.google.com/?cid=2219219932235197697&g_mp=CiVnb29nbGUubWFwcy5wbGFjZXMudjEuUGxhY2VzLkdldFBsYWNlEAAYASAA&hl=en-US&source=apiv3",
	OpeningHours: []string{
		"Monday: Closed",
		"Tuesday: Closed",
		"Wednesday: 5:00 – 10:00 PM",
		"Thursday: 5:00 – 10:00 PM",
		"Friday: 5:00 – 10:00 PM",
		"Saturday: 5:00 – 10:00 PM",
		"Sunday: Closed",
	},
	ContactFirstName:   "Sanchez",
	ContactLastName:    "O'Leary",
	ContactPhone:       "4156038101",
	PosSystem:          "Shopify",
	SoleProprietorship: "No",
	TippingPolicy:      "N/A - our employees do not receive tips",
	TippingDivision:    "All tips are pooled and divided between the team",
	TableCoverage:      "Table coverage is managed differently (e.g. rotating, team service, etc.)",
	ServiceStations:    3,
	TabletModel:        "We do not have a tablet accessible near register",
	MessagingService:   "We do not currently use a messaging service",
	Reference:          "casCAS",
}

var TEST_LOCATION_2 = structs.Location{
	ID:          2,
	GoogleID:    "ChIJ92WDBbuAhYARQZ1TXrfhXv0",
	OwnerID:     TEST_USER_2.Id,
	Name:        "McDonald's",
	Description: "casascsa",
	Type:        "Fast Food Restaurant",
	Approval:    &testTrue,
	Street:      "1100 Fillmore Street",
	City:        "San Francisco",
	State:       "California",
	ZIP:         "94102",
	Lat:         37.7798713,
	Lng:         -122.4317172,
	Phone:       "4156038101",
	Email:       "sanchez@oleary.com",
	AdminPhone:  "4156038101",
	AdminEmail:  "sanchez@oleary.com",
	Website:     "https://www.mcdonalds.com/us/en-us/location/CA/SAN-FRANCISCO/1100-FILLMORE-ST/10162.html?cid=RF:YXT:GMB::Clicks",
	ImageURL:    "https://upload.wikimedia.org/wikipedia/commons/thumb/0/05/McDonald%27s_square_2020.svg/960px-McDonald%27s_square_2020.svg.png",
	Rating:      3.4,
	MapsPage:    "https://maps.google.com/?cid=18257278117084372289&g_mp=CiVnb29nbGUubWFwcy5wbGFjZXMudjEuUGxhY2VzLkdldFBsYWNlEAAYASAA&hl=en-US&source=apiv3",
	OpeningHours: []string{
		"Monday: 7:30 AM – 10:00 PM",
		"Tuesday: 6:00 AM – 10:00 PM",
		"Wednesday: 6:00 AM – 10:00 PM",
		"Thursday: 6:00 AM – 10:00 PM",
		"Friday: 6:00 AM – 10:00 PM",
		"Saturday: 6:00 AM – 10:00 PM",
		"Sunday: 6:00 AM – 10:00 PM",
	},
	ContactFirstName:   "Sanchez",
	ContactLastName:    "O'Leary",
	ContactPhone:       "4156038101",
	PosSystem:          "Shopify",
	SoleProprietorship: "No",
	TippingPolicy:      "Both (depends on party size or situation)",
	TippingDivision:    "csasacs",
	TableCoverage:      "Table coverage is managed differently (e.g. rotating, team service, etc.)",
	ServiceStations:    8,
	TabletModel:        "Android tablet",
	MessagingService:   "We do not currently use a messaging service",
	Reference:          "acsacsa",
}

var TEST_LOCATION_2A = structs.Location{
	ID:          2,
	GoogleID:    "ChIJ92WDBbuAhYARQZ1TXrfhXv0",
	OwnerID:     TEST_USER_2.Id,
	Name:        "McDonald's",
	Description: "test changes",
	Type:        "Fast Food Restaurant",
	Approval:    &testTrue,
	Street:      "1100 Fillmore Street",
	City:        "San Francisco",
	State:       "California",
	ZIP:         "94102",
	Lat:         37.7798713,
	Lng:         -122.4317172,
	Phone:       "4156038101",
	Email:       "sanchez@oleary.com",
	AdminPhone:  "4156038101",
	AdminEmail:  "sanchez@oleary.com",
	Website:     "https://www.mcdonalds.com/us/en-us/location/CA/SAN-FRANCISCO/1100-FILLMORE-ST/10162.html?cid=RF:YXT:GMB::Clicks",
	ImageURL:    "https://upload.wikimedia.org/wikipedia/commons/thumb/0/05/McDonald%27s_square_2020.svg/960px-McDonald%27s_square_2020.svg.png",
	Rating:      3.4,
	MapsPage:    "https://maps.google.com/?cid=18257278117084372289&g_mp=CiVnb29nbGUubWFwcy5wbGFjZXMudjEuUGxhY2VzLkdldFBsYWNlEAAYASAA&hl=en-US&source=apiv3",
	OpeningHours: []string{
		"Monday: 8:30 AM – 10:00 PM",
		"Tuesday: 6:00 AM – 10:00 PM",
		"Wednesday: 6:00 AM – 10:00 PM",
		"Thursday: 7:00 AM – 10:00 PM",
		"Friday: 6:00 AM – 10:00 PM",
		"Saturday: 6:00 AM – 10:00 PM",
		"Sunday: 6:00 AM – 10:00 PM",
	},
	ContactFirstName:   "Sanchez",
	ContactLastName:    "O'Leary",
	ContactPhone:       "4156038101",
	PosSystem:          "Square",
	SoleProprietorship: "No",
	TippingPolicy:      "Both (depends on party size or situation)",
	TippingDivision:    "fair and balanced",
	TableCoverage:      "Table coverage is managed differently (e.g. rotating, team service, etc.)",
	ServiceStations:    8,
	TabletModel:        "Android tablet",
	MessagingService:   "We do not currently use a messaging service",
	Reference:          "some guy",
}

var TEST_LOCATIONS = []structs.Location{TEST_LOCATION_1, TEST_LOCATION_2A}

var TEST_CONTACT_1 = structs.Contact{
	Id:         1,
	Owner:      TEST_USER_1.Id,
	Name:       "test_contact_1",
	Address:    "0x7e571",
	IsFavorite: false,
}

var TEST_CONTACT_2 = structs.Contact{
	Id:         2,
	Owner:      TEST_USER_2.Id,
	Name:       "test_contact_2",
	Address:    "0x7e572",
	IsFavorite: true,
}

var TEST_CONTACT_2A = structs.Contact{
	Id:         2,
	Owner:      TEST_USER_2.Id,
	Name:       "test_contact_2a",
	Address:    "0x7e572a",
	IsFavorite: true,
}

var TEST_CONTACTS = []structs.Contact{TEST_CONTACT_1, TEST_CONTACT_2A}

var AppDb *db.AppDB
var TestServer *httptest.Server

func TestApp(t *testing.T) {
	GroupControllers(t)
	GroupHandlers(t)
}

func GroupControllers(t *testing.T) {
	adb, err := db.PgxDB("test_app_controllers")
	if err != nil {
		t.Fatalf("error establishing db connection: %s\n", err)
	}
	defer adb.Close()

	timeString := time.Now().Format(time.RFC3339)
	appLogger, err := logger.New(fmt.Sprintf("./logs/test/app/app_test_%s.log", timeString), "APP_TEST: ")
	if err != nil {
		t.Fatalf("error initializing app logger: %s\n", err)
	}
	defer appLogger.Close()

	defer appLogger.Close()
	AppDb = db.App(adb, appLogger)
	err = AppDb.CreateTables()
	if err != nil {
		t.Fatalf("error creating app db tables for controllers: %s\n", err)
	}

	usersControllers := t.Run("user controllers group", GroupUsersControllers)
	if !usersControllers {
		t.Fatal("users controllers group failed")
	}
	walletsControllers := t.Run("wallets controllers group", GroupWalletsControllers)
	locationControllers := t.Run("location controllers group", GroupLocationControllers)
	contactsControllers := t.Run("contacts controllers group", GroupContactsControllers)
	if !walletsControllers || !locationControllers || !contactsControllers {
		t.Error("wallets, locations, or contacts controllers group failed")
	}

	adminControllers := t.Run("admin controllers group", GroupAdminControllers)
	if !adminControllers {
		t.Error("admin controllers failed group")
	}
}

func GroupHandlers(t *testing.T) {
	adb, err := db.PgxDB("test_app_handlers")
	if err != nil {
		t.Fatalf("error establishing db connection: %s\n", err)
	}
	defer adb.Close()

	timeString := time.Now().Format(time.RFC3339)
	appLogger, err := logger.New(fmt.Sprintf("./logs/test/app/app_test_%s.log", timeString), "APP_TEST: ")
	if err != nil {
		t.Fatalf("error initializing app logger: %s\n", err)
	}
	defer appLogger.Close()

	appHandlersDb := db.App(adb, appLogger)
	err = appHandlersDb.CreateTables()
	if err != nil {
		t.Fatalf("error creating app db tables for handlers: %s\n", err)
	}

	testRouter := chi.NewRouter()
	appService := handlers.NewAppService(appHandlersDb, appLogger, nil)
	Spoofer = NewContextSpoofer("userDid", TEST_USER_1.Id)

	testRouter.Use(middleware.AuthMiddleware)
	testRouter.Use(Spoofer.Middleware)

	router.AddUserRoutes(testRouter, appService)
	router.AddWalletRoutes(testRouter, appService)
	router.AddLocationRoutes(testRouter, appService)
	router.AddContactRoutes(testRouter, appService)

	TestServer = httptest.NewServer(testRouter)
	defer TestServer.Close()

	usersHandlers := t.Run("users handlers group", GroupUsersHandlers)
	if !usersHandlers {
		t.Fatal("users handlers group failed")
	}

	walletsHandlers := t.Run("wallets handlers group", GroupWalletsHandlers)
	if !walletsHandlers {
		t.Error("wallets handlers group failed")
	}

	locationHandlers := t.Run("location handlers group", GroupLocationHandlers)
	if !locationHandlers {
		t.Fatal("location handlers group failed")
	}

	contactsHandlers := t.Run("contacts handlers group", GroupContactsHandlers)
	if !contactsHandlers {
		t.Fatal("contacts handlers group failed")
	}

	adminHandlers := t.Run("admin handlers group", GroupAdminHandlers)
	if !adminHandlers {
		t.Fatal("admin handlers group failed")
	}
}
