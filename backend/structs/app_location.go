package structs

// TODO SANCHEZ: Define the Location struct with appropriate fields, this is the serializer

type Location struct {
	ID           uint        `json:"id"`
	GoogleID     string      `json:"google_id"`
	OwnerID      string      `json:"owner_id"`
	Name         string      `json:"name"`
	Description  string      `json:"description"`
	Type         string      `json:"type"`
	Approval     bool        `json:"approval"`
	Street       string      `json:"street"`
	City         string      `json:"city"`
	State        string      `json:"state"`
	ZIP          string      `json:"zip"`
	Lat          float64     `json:"lat"`
	Lng          float64     `json:"lng"`
	Phone        string      `json:"phone"`
	Email        string      `json:"email"`
	Website      string      `json:"website"`
	ImageURL     string      `json:"image_url"`
	Rating       float64     `json:"rating"`
	MapsPage     string      `json:"maps_page"`
	OpeningHours [][2]string `json:"opening_hours"`
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
