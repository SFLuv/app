package structs

// TODO SANCHEZ: Define the Location struct with appropriate fields, this is the serializer

/*type Location struct {
	ID           uint         `json:"id"`
	GoogleID     string       `json:"google_id"`
	OwnerID      string       `json:"owner_id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Type         string       `json:"type"`
	Approval     bool         `json:"approval"`
	Street       string       `json:"street"`
	City         string       `json:"city"`
	State        string       `json:"state"`
	ZIP          string       `json:"zip"`
	Lat          float64      `json:"lat"`
	Lng          float64      `json:"lng"`
	Phone        string       `json:"phone"`
	Email        string       `json:"email"`
	Website      string       `json:"website"`
	ImageURL     string       `json:"image_url"`
	Rating       float64      `json:"rating"`
	MapsPage     string       `json:"maps_page"`
	OpeningHours [][2]float64 `json:"opening_hours"`
}*/

type Location struct {
	ID                 uint     `json:"id"`
	GoogleID           string   `json:"google_id"`
	OwnerID            string   `json:"owner_id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Type               string   `json:"type"`
	Approval           *bool    `json:"approval"`
	Street             string   `json:"street"`
	City               string   `json:"city"`
	State              string   `json:"state"`
	ZIP                string   `json:"zip"`
	Lat                float64  `json:"lat"`
	Lng                float64  `json:"lng"`
	Phone              string   `json:"phone"`
	Email              string   `json:"email"`
	AdminPhone         string   `json:"admin_phone"`
	AdminEmail         string   `json:"admin_email"`
	Website            string   `json:"website"`
	ImageURL           string   `json:"image_url"`
	Rating             float64  `json:"rating"`
	MapsPage           string   `json:"maps_page"`
	OpeningHours       []string `json:"opening_hours"`
	ContactFirstName   string   `json:"contact_firstname"`
	ContactLastName    string   `json:"contact_lastname"`
	ContactPhone       string   `json:"contact_phone"`
	PosSystem          string   `json:"pos_system"`
	SoleProprietorship string   `json:"sole_proprietorship"`
	TippingPolicy      string   `json:"tipping_policy"`
	TippingDivision    string   `json:"tipping_division"`
	TableCoverage      string   `json:"table_coverage"`
	ServiceStations    int      `json:"service_stations"`
	TabletModel        string   `json:"tablet_model"`
	MessagingService   string   `json:"messaging_service"`
	Reference          string   `json:"reference"`
}

type PublicLocation struct {
	ID           uint     `json:"id"`
	GoogleID     string   `json:"google_id"`
	Name         string   `json:"name"`
	Approval     bool     `json:"approval"`
	Description  string   `json:"description"`
	Type         string   `json:"type"`
	Street       string   `json:"street"`
	City         string   `json:"city"`
	State        string   `json:"state"`
	ZIP          string   `json:"zip"`
	Lat          float64  `json:"lat"`
	Lng          float64  `json:"lng"`
	Phone        string   `json:"phone"`
	Email        string   `json:"email"`
	Website      string   `json:"website"`
	ImageURL     string   `json:"image_url"`
	Rating       float64  `json:"rating"`
	MapsPage     string   `json:"maps_page"`
	OpeningHours []string `json:"opening_hours"`
}

type LocationsPageRequest struct {
	Page  uint
	Count uint
}

type AuthedLocationResponse struct {
}

type LocationResponse struct {
	Name    string          `json:"name"`
	Address LocationAddress `json:"address"`
}

type LocationAddress struct {
	Street string `json:"street"`
}
