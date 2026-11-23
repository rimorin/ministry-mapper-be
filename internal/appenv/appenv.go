// Package appenv tells the rest of the backend which environment it is
// running in, from the APP_ENV variable.
package appenv

import (
	"os"
	"strings"
)

// Production is the environment real congregations use. It is also what a
// deployment without APP_ENV is taken to be, so that a forgotten variable
// never holds back real email.
const Production = "production"

// Name returns the environment: "production", "staging", "development" or
// whatever APP_ENV names, in lower case.
func Name() string {
	if name := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))); name != "" {
		return name
	}
	return Production
}

// IsProduction reports whether this is the production environment.
func IsProduction() bool {
	return Name() == Production
}

// MarkSubject puts the environment's name in front of an email subject
// outside production, so a test email cannot be taken for a real one.
func MarkSubject(subject string) string {
	if IsProduction() {
		return subject
	}
	return "[" + strings.ToUpper(Name()) + "] " + subject
}

// MayEmail reports whether an address may be sent email. In production every
// address may. Elsewhere only those in EMAIL_ALLOWLIST, a comma-separated
// list of addresses and "@domain" entries; an empty list means nobody.
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
