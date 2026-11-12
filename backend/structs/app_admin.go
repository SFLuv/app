package structs

type UpdateLocationApprovalRequest struct {
	Id       uint  `json:"id"`
	Approval *bool `json:"approval"`
	// Setup approves the location but keeps it unlisted; see
	// Location.InSetup. Ignored unless Approval is true, and omitted by
	// every caller that only approves, which lists the location.
	Setup bool `json:"setup"`
}
