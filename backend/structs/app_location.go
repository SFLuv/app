package structs

// TODO SANCHEZ: Define the LocationRequest struct with appropriate fields, this is the serializer

type LocationRequest struct {
	ID          uint    `json:"id"`
	GoogleID    string  `json:"google_id"`
	OwnerID     string  `json:"owner_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Type        string  `json:"type"`
	Approval    bool    `json:"approval"`
	Street      string  `json:"street"`
	City        string  `json:"city"`
	State       string  `json:"state"`
	ZIP         string  `json:"zip"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	Phone       string  `json:"phone"`
	Email       string  `json:"email"`
	Website     string  `json:"website"`
	ImageURL    string  `json:"image_url"`
	Rating      float64 `json:"rating"`
	MapsPage    string  `json:"maps_page"`
}

type LocationsPageRequest struct {
	Page  uint
	Count uint
}

type Location struct {
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
