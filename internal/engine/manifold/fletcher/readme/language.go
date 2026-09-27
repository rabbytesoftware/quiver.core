package readme

import "unicode"

const defaultReadmeName = "README.md"

func EnglishReadmeNames() []string {
	return []string{"README.en.md", "README_EN.md", "README.english.md"}
}

func knownReadmeNames() []string {
	return []string{
		defaultReadmeName,
		"README.en.md",
		"README_EN.md",
		"Readme.md",
		"readme.md",
	}
}

func isCJKRune(
	r rune,
) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF:
		return true
	case r >= 0x3040 && r <= 0x30FF:
		return true
	case r >= 0xAC00 && r <= 0xD7A3:
		return true
	default:
		return false
	}
}

func IsCJKDominant(
	raw []byte,
) bool {
	var cjk, letters int
	for _, r := range string(raw) {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if isCJKRune(r) {
			cjk++
		}
	}
	if letters == 0 {
		return false
	}
	return cjk*2 > letters
}

func firstKnownReadme(
	byName map[string][]byte,
) []byte {
	for _, name := range knownReadmeNames() {
		if raw, ok := byName[name]; ok {
			return raw
		}
	}
	return nil
}

func SelectReadme(
	byName map[string][]byte,
) []byte {
	def, hasDefault := byName[defaultReadmeName]
	if hasDefault && !IsCJKDominant(def) {
		return def
	}
	for _, name := range EnglishReadmeNames() {
		if alt, ok := byName[name]; ok {
			return alt
		}
	}
	if hasDefault {
		return def
	}
	return firstKnownReadme(byName)
}
