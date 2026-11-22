package cairnpost

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// longestWait is the longest wait an answer is believed to ask for,
// about 31 years. No server means more, and it is the most seconds the
// Retry-After header can say in the nine digits read of it.
const longestWait = 999_999_999 * time.Second

// waitOf is how long an answer asks to wait, zero when it does not say.
// Retry-After is seconds from CairnPost, and may be a date from a proxy.
// Said more than once, its values are read as one with a comma between
// them, as HTTP says to read them and as the other clients are handed
// them: two waits said so are none.
func waitOf(res *http.Response) time.Duration {
	return waitIn(strings.Join(res.Header.Values("Retry-After"), ", "), time.Now())
}

// waitIn reads a Retry-After value. It is a wait in two forms only: 1
// to 9 digits, which are seconds, and a date written exactly as HTTP
// writes one now ("Sun, 06 Nov 1994 08:49:37 GMT"), which is the time to
// wait until. A sign, a fraction, an older form of a date or a date that
// never comes is no wait: the Node and Python clients read the same.
func waitIn(after string, now time.Time) time.Duration {
	after = strings.Trim(after, around)
	if n, digits := secondsOf(after); digits {
		if len(after) > 9 {
			return 0
		}
		return time.Duration(n) * time.Second
	}
	// Parse alone takes other spellings, and a 31 Feb as a day in March.
	at, err := time.Parse(http.TimeFormat, after)
	if err != nil || at.Format(http.TimeFormat) != after {
		return 0
	}
	// Sub answers the longest time there is for one further off.
	left := at.Sub(now)
	if left <= 0 || left > longestWait {
		return 0
	}
	// A part of a second is a whole one: a caller never comes back early.
	return (left + time.Second - 1).Truncate(time.Second)
}

// waitSaid reads the wait one email of a batch was answered with: a
// JSON number of whole seconds, however it is written (30, 30.0, 3e1).
// Anything else there is no wait.
func waitSaid(raw json.RawMessage) time.Duration {
	n, err := strconv.ParseFloat(string(raw), 64)
	if err != nil || n < 1 || n > longestWait.Seconds() || n != math.Trunc(n) {
		return 0
	}
	return time.Duration(n) * time.Second
}
