package util

// MaskToken masks a token by showing first 10 and last 9 characters.
// Example: "ff_08be239abcdefghije13757b8e" -> "ff_08be239***************e13757b8e"
func MaskToken(token string) string {
	if len(token) <= 18 {
		return token
	}
	first := token[:10]
	last := token[len(token)-9:]
	return first + "***************" + last
}
