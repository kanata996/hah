package reqx

import "strings"

func pathHasWildcard(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	name = strings.TrimSpace(name)
	if pattern == "" || name == "" {
		return false
	}

	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '{' {
			continue
		}

		end := strings.IndexByte(pattern[i+1:], '}')
		if end < 0 {
			break
		}

		if normalizePathWildcardToken(pattern[i+1:i+1+end]) == name {
			return true
		}

		i += end + 1
	}

	return false
}

func normalizePathWildcardToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.TrimSuffix(token, "...")
	token, _, _ = strings.Cut(token, ":")
	token = strings.TrimSpace(token)
	if token == "$" {
		return ""
	}
	return token
}
