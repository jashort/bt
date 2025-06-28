package internal

import (
	"github.com/markusmobius/go-dateparser"
	"strings"
	"time"
)

// ParseTimestamp tries to pass the given string as a timestamp in the current (system) timezone
// If the string is empty, the current time is used.
// baseTime is used to determine the current timezone. (Pass in time.Now())
func ParseTimestamp(at string, baseTime time.Time) (time.Time, error) {
	var now time.Time
	if strings.TrimSpace(at) == "" {
		now = time.Now()
	} else {
		cfg := &dateparser.Configuration{
			CurrentTime: baseTime,
		}
		dps, err := dateparser.Parse(cfg, at)
		if err != nil {
			return now, err
		}
		now = dps.Time
	}
	return now, nil
}
