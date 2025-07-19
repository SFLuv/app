package structs

// TODO SANCHEZ: Define the LocationRequest struct with appropriate fields, this is the serializer

type LocationRequest struct {
	Name        string `json:"name"`
	GoogleID    string `json:"googleid"`
	Description string `json:"description"`
	ID          uint   `json:"id"`
}

type Location struct {
}

type AuthedLocationResponse struct {
}

type LocationResponse struct {
	Name    string          `json:"name"`
	Address LocationAddress `json"address"`
}

type LocationAddress struct {
	Street string `json:"street"`
}
