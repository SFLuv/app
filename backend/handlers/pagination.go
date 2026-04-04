package handlers

import (
	"net/url"
	"strconv"
)

func parsePageAndCount(params url.Values, defaultCount, maxCount int) (int, int) {
	page, err := strconv.Atoi(params.Get("page"))
	if err != nil || page < 0 {
		page = 0
	}

	count, err := strconv.Atoi(params.Get("count"))
	if err != nil || count <= 0 {
		count = defaultCount
	}
	if maxCount > 0 && count > maxCount {
		count = maxCount
	}

	return page, count
}
