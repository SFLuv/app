package structs

import "time"

// Beta features: surfaces that exist in the build but are shown only to people
// who have been let in, one feature at a time.
//
// The catalog is code rather than data, which is the opposite of how credential
// types work and for a specific reason: a beta flag is only meaningful if some
// code path reads it. A row naming a feature nothing gates would grant access
// to nothing, and the admin panel would offer it as though it did something. So
// features are declared here, beside the handlers that enforce them, and the
// database holds only who may use them.
const (
	// BetaFeatureBulletin is the neighbourhood Bulletin board.
	BetaFeatureBulletin = "bulletin"
	// BetaFeatureChat is direct messages. Distinct from the bulletin because a
	// chat has a second party: letting somebody post is a decision about them
	// alone, letting somebody message is a decision that reaches whoever they
	// message.
	BetaFeatureChat = "chat"
)

// BetaFeatureDefinition describes one feature for the admin panel.
type BetaFeatureDefinition struct {
	Id    string `json:"id"`
	Label string `json:"label"`
	// Description says what granting it actually turns on, because "enable
	// chat for this person" is not answerable without knowing whether that
	// also lets other people message them.
	Description string `json:"description"`
	// EnvEnabled is the server-wide switch for this feature. False means no
	// grant has any effect, which the panel has to show or an admin will grant
	// access and watch nothing happen.
	EnvEnabled bool `json:"env_enabled"`
	// EnvVar is named so the panel can say which switch to set.
	EnvVar string `json:"env_var"`
	// Members is how many people currently hold it.
	Members int `json:"members"`
}

// BetaFeatureCatalog is every feature the admin panel can grant.
func BetaFeatureCatalog() []BetaFeatureDefinition {
	return []BetaFeatureDefinition{
		{
			Id:          BetaFeatureBulletin,
			Label:       "Bulletin board",
			Description: "The neighbourhood board: reading the feed, posting, photos, and trade feedback. Granting it affects only this person.",
			EnvVar:      EnvBetaBulletin,
		},
		{
			Id:    BetaFeatureChat,
			Label: "Direct messages",
			Description: "One-to-one chats, including payments inside a chat. " +
				"Both people need it: a chat cannot be started with somebody who does not have it, and revoking it leaves existing chats in place but closed to them.",
			EnvVar: EnvBetaChat,
		},
	}
}

// Env switches for the two features. A grant is necessary but not sufficient —
// these are the master switches, so a feature can be withdrawn from everybody
// at once without touching the grants, and the grants are still there when it
// comes back.
const (
	EnvBetaBulletin = "BULLETIN_ENABLED"
	EnvBetaChat     = "CHAT_ENABLED"
)

// IsBetaFeature reports whether an id is one we know. Guards the admin
// endpoints so a typo cannot create a grant for a feature nothing reads.
func IsBetaFeature(id string) bool {
	for _, feature := range BetaFeatureCatalog() {
		if feature.Id == id {
			return true
		}
	}
	return false
}

// BetaMember is one person's standing with one feature, for the admin panel.
//
// Revoked members are listed rather than dropped. The row is the history, and
// it is also what makes re-granting restore somebody to exactly where they
// were instead of treating them as new.
type BetaMember struct {
	UserId    string     `json:"user_id"`
	Email     string     `json:"email,omitempty"`
	Name      string     `json:"name,omitempty"`
	GrantedAt time.Time  `json:"granted_at"`
	GrantedBy string     `json:"granted_by,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	RevokedBy string     `json:"revoked_by,omitempty"`
	// Active is the answer the gates use: granted and not since revoked.
	Active bool `json:"active"`
}
