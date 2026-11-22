package cairnpost

import (
	"net/http"
	"strconv"
	"strings"
)

// signed reads the signature headers under whichever names they came:
// the names in any case, each value without the space, tabs and line
// breaks around it. net/http hands a header on with its names in one form
// and its values bare, and Get finds only that form; a header built by
// hand, from a queue or a function's event, may come in small letters
// and with space around.
func signed(h http.Header) (id, stamp, signatures string) {
	said, under := map[string]string{}, map[string]string{}
	for name, values := range h {
		lower := strings.ToLower(name)
		if was, twice := under[lower]; len(values) == 0 || twice && !before(name, was) {
			continue
		}
		under[lower], said[lower] = name, strings.Trim(values[0], around)
	}
	for _, prefix := range []string{"webhook-", "svix-"} {
		id, stamp, signatures = said[prefix+"id"], said[prefix+"timestamp"], said[prefix+"signature"]
		if id != "" && stamp != "" && signatures != "" {
			return id, stamp, signatures
		}
	}
	return id, stamp, signatures
}

// before says which of two names for one header is read when it comes
// under both: the form net/http writes, as Get would, and otherwise the
// first in order, so that the answer is the same every time.
func before(name, other string) bool {
	if ours, theirs := name == http.CanonicalHeaderKey(name), other == http.CanonicalHeaderKey(other); ours != theirs {
		return ours
	}
	return name < other
}

// isAround says a character is one that stands between two signatures.
func isAround(r rune) bool { return strings.ContainsRune(around, r) }

// secondsOf reads a webhook's timestamp: one to fifteen digits and
// nothing else. strconv alone would take a sign, and a time longer than
// any there is.
func secondsOf(stamp string) (int64, bool) {
	if len(stamp) == 0 || len(stamp) > 15 {
		return 0, false
	}
	for i := 0; i < len(stamp); i++ {
		if stamp[i] < '0' || stamp[i] > '9' {
			return 0, false
		}
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	return seconds, err == nil
}
