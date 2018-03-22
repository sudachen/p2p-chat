package chat

import "time"

func done(q chan struct{}) bool {
	select {
	case <-q:
		return true
	default:
		return false
	}
}

func done2(q chan struct{}, t <-chan time.Time) bool {
	select {
	case <-q:
		return true
	case <-t:
		return false
	}
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1]
		}
	}
	return s
}
