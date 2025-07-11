package structs

// TODO SANCHEZ: Define the MerchantRequest struct with appropriate fields, this is the serializer

type MerchantRequest struct {
	Name        string `json:"name"`
	GoogleID    string `json:"googleid"`
	Description string `json:"description"`
	ID          uint   `json:"id"`
}
