package main

import (
	"errors"
	"os"
	"strings"
	"time"
)

func humanizeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateTime)
}

func parseAcRate(acRate any) (string, error) {
	acRateStr, ok := acRate.(string)
	if !ok {
		return "", errors.New("acRate is not a string")
	}
	return strings.TrimSuffix(acRateStr, "%"), nil
}

func fileExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		// file apparently exists
		return true, nil
	} else {
		// got error, let's see
		if errors.Is(err, os.ErrNotExist) {
			// file not exists, so no actual error here
			return false, nil
		} else {
			// other error
			return false, err
		}
	}
}
