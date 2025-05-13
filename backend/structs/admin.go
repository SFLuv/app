package structs

type Admin struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Limit       int    `json:"limit"`
	Refresh     int    `json:"refresh"`
	Balance     int    `json:"balance"`
	LastRefresh int    `json:"last_refresh"`
}
