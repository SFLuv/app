package utils

import "net/http"

func GetDid(r *http.Request) *string {
	userDid, ok := r.Context().Value("userDid").(string)
	if !ok {
		return nil
	}

	return &userDid
}
