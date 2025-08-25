package structs

type Contact struct {
	Id         int    `json:"id"`
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	IsFavorite bool   `json:"is_favorite"`
}
