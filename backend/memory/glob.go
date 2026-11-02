package memory

// match reports whether s matches the Redis glob pattern: * ? [abc] [^abc] [a-z] and \ escapes.
func match(pattern, s string) bool {
	for pattern != "" {
		if pattern[0] == '*' {
			return matchStar(pattern, s)
		}
		if s == "" {
			return false
		}
		var ok bool
		pattern, ok = matchOne(pattern, s[0])
		if !ok {
			return false
		}
		s = s[1:]
	}
	return s == ""
}

// matchStar matches a pattern that starts with '*' against every suffix of s.
func matchStar(pattern, s string) bool {
	for pattern != "" && pattern[0] == '*' {
		pattern = pattern[1:]
	}
	if pattern == "" {
		return true
	}
	for i := 0; i <= len(s); i++ {
		if match(pattern, s[i:]) {
			return true
		}
	}
	return false
}

// matchOne matches c against the first token of pattern (not '*') and returns the rest.
func matchOne(pattern string, c byte) (string, bool) {
	switch {
	case pattern[0] == '?':
		return pattern[1:], true
	case pattern[0] == '[':
		return matchClass(pattern[1:], c)
	case pattern[0] == '\\' && len(pattern) > 1:
		return pattern[2:], pattern[1] == c
	default:
		return pattern[1:], pattern[0] == c
	}
}

// matchClass matches c against the class that starts right after '[' and returns the pattern
// after the closing ']'. A class without ']' runs to the end of the pattern.
func matchClass(pattern string, c byte) (string, bool) {
	negate := pattern != "" && pattern[0] == '^'
	if negate {
		pattern = pattern[1:]
	}
	found := false
	for pattern != "" && pattern[0] != ']' {
		var hit bool
		pattern, hit = classItem(pattern, c)
		found = found || hit
	}
	if pattern != "" {
		pattern = pattern[1:]
	}
	return pattern, found != negate
}

// classItem matches c against one item of a class: an escaped byte, a range or a byte.
func classItem(pattern string, c byte) (string, bool) {
	switch {
	case pattern[0] == '\\' && len(pattern) > 1:
		return pattern[2:], pattern[1] == c
	case len(pattern) > 2 && pattern[1] == '-': // like Redis, "a-]" is a range up to ']'
		lo, hi := min(pattern[0], pattern[2]), max(pattern[0], pattern[2])
		return pattern[3:], lo <= c && c <= hi
	default:
		return pattern[1:], pattern[0] == c
	}
}
