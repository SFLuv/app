package utils

import "time"

func GetMostRecentReset(from *time.Time, resetPeriod int) *time.Time {
	if resetPeriod == 0 {
		return from
	}

	refreshPeriod := time.Hour * time.Duration(resetPeriod*24)
	newTime := *from
	now := time.Now()
	for newTime.Add(refreshPeriod).Before(now) {
		newTime = newTime.Add(refreshPeriod)
	}

	return &newTime
}
