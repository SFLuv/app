package structs

type Contact struct {
	Id         int    `json:"id"`
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	IsFavorite bool   `json:"is_favorite"`
	// Notes the owner wrote about this person. Private to them — never shown to
	// the contact, and not part of anybody's public profile.
	Notes string `json:"notes"`
	// The SFLuv account this address belongs to, when it belongs to one.
	//
	// Resolved by the server when the contact is saved, not supplied by the
	// client, and deliberately not surfaced as a fact about the contact: the
	// app never says "this person has an account". What it does is let the
	// new-message picker offer them by the name you saved them under, which is
	// the only place the link is visible at all.
	UserId *string `json:"user_id,omitempty"`
	// Whether a picture has been uploaded, so the list knows to fetch one
	// without a request per contact that has none.
	HasAvatar bool `json:"has_avatar"`
}

// ContactPatch is a partial update to a contact card. Every field is optional,
// so the detail screen can save one thing without resending the rest.
type ContactPatch struct {
	Name       *string `json:"name"`
	Address    *string `json:"address"`
	IsFavorite *bool   `json:"is_favorite"`
	Notes      *string `json:"notes"`
}
