package domain

import "strings"

const numberedDeviceStemLen = 4

func IsWindowsReservedName(
	name string,
) bool {
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.ToUpper(stem)
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}

	if len(stem) != numberedDeviceStemLen || (!strings.HasPrefix(stem, "COM") && !strings.HasPrefix(stem, "LPT")) {
		return false
	}

	return stem[3] >= '1' && stem[3] <= '9'
}
