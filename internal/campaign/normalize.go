package campaign

import (
	"strings"
	"unicode"
)

const (
	maxEmailLength = 254
	maxLocalLength = 64
)

func NormalizeEmail(raw string) (string, bool) {
	email := strings.TrimSpace(raw)
	if len(email) == 0 || len(email) > maxEmailLength || !asciiPrintable(email) {
		return "", false
	}
	if strings.Count(email, "@") != 1 {
		return "", false
	}
	local, domain, _ := strings.Cut(email, "@")
	if !validLocal(local) || !validDomain(domain) {
		return "", false
	}
	return strings.ToLower(email), true
}

func validLocal(local string) bool {
	if len(local) == 0 || len(local) > maxLocalLength || local[0] == '.' || local[len(local)-1] == '.' || strings.Contains(local, "..") {
		return false
	}
	for _, r := range local {
		if isASCIIAlphaNumeric(r) || strings.ContainsRune(".!#$%&'*+-/=?^_`{|}~", r) {
			continue
		}
		return false
	}
	return true
}

func validDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 || strings.Contains(domain, "..") {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !isASCIIAlphaNumeric(r) && r != '-' {
				return false
			}
		}
	}
	return true
}

func asciiPrintable(value string) bool {
	for _, r := range value {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}

func isASCIIAlphaNumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func UsableName(raw string) (string, bool) {
	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, raw))
	name = strings.Join(strings.Fields(name), " ")
	return name, name != ""
}
