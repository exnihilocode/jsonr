package jsonr

import (
	"fmt"
	"strings"
	"time"
)

func decodeTime(val string) (time.Time, error) {
	s := strings.TrimSpace(val)
	if s == "" {
		return time.Time{}, fmt.Errorf("jsonr: empty time string")
	}
	if tm, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return tm, nil
	}
	if tm, err := time.Parse(time.RFC3339, s); err == nil {
		return tm, nil
	}
	utc := time.UTC
	layoutsUTC := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"15:04:05",
		"15:04",
	}
	for _, layout := range layoutsUTC {
		if tm, err := time.ParseInLocation(layout, s, utc); err == nil {
			return tm, nil
		}
	}
	return time.Time{}, fmt.Errorf("jsonr: cannot parse time string %q (supported: RFC3339/RFC3339Nano, YYYY-MM-DD, datetime without offset in UTC, HH:MM:SS, HH:MM)", s)
}
