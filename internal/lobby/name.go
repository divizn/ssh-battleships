package lobby

import "strings"

// NameLimit is what the layout can print without pushing a column out of shape.
const NameLimit = 12

// CleanName reduces whatever a player supplied to something safe to print in somebody
// else's terminal. Names arrive from the ssh username and from a prompt, both of which are
// entirely under the connecting player's control, so escape sequences and control
// characters have to go before the name is ever rendered.
func CleanName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r > 127 || b.Len() >= NameLimit {
			continue
		}
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
