package shared

import "regexp"

type URL string

func (u URL) String() string {
	return string(u)
}

func (u URL) IsValid() bool {
	// Regular expression to validate URL format
	// Matches:
	// - Protocol (http/https/ftp)
	// - Domain name or IP address
	// - Optional port number
	// - Optional path
	// - Optional query parameters
	// - Optional fragment
	urlRegex := regexp.MustCompile(`^(https?|ftp):\/\/[^\s/$.?#].[^\s]*$`)
	return urlRegex.MatchString(string(u))
}
