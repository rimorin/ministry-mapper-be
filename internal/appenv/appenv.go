// Package appenv reports the environment named by APP_ENV.
package appenv

import (
	"os"
	"strings"
)

// Production is the default when APP_ENV is unset.
const Production = "production"

// Name returns the environment in lower case.
func Name() string {
	if name := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))); name != "" {
		return name
	}
	return Production
}

func IsProduction() bool {
	return Name() == Production
}

// MarkSubject prefixes a subject with the environment outside production.
func MarkSubject(subject string) string {
	if IsProduction() {
		return subject
	}
	return "[" + strings.ToUpper(Name()) + "] " + subject
}

// MayEmail reports whether an address is in EMAIL_ALLOWLIST (addresses or
// @domains). Always true in production.
func MayEmail(address string) bool {
	if IsProduction() {
		return true
	}
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return false
	}
	for _, entry := range strings.Split(os.Getenv("EMAIL_ALLOWLIST"), ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if entry == address || (strings.HasPrefix(entry, "@") && strings.HasSuffix(address, entry)) {
			return true
		}
	}
	return false
}
