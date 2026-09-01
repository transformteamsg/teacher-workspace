package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/String-sg/teacher-workspace/server/internal/config"
)

// pgIdentity is the caller identity PG needs to authorise a /staff/* request.
type pgIdentity struct {
	Subject    string
	SchoolCode string
	Roles      []string
	// PG rejects a token without effective_role, and requires ATTR_PG_USER in
	// attributes for anyone who is not a school leader.
	EffectiveRole string
	Attributes    []string
}

const (
	envPGStubSubject    = "TW_API_PROXY_POSTS_STUB_SUBJECT"
	envPGStubSchoolCode = "TW_API_PROXY_POSTS_STUB_SCHOOL_CODE"
	envPGStubRoles      = "TW_API_PROXY_POSTS_STUB_ROLES"
	envPGStubEffective  = "TW_API_PROXY_POSTS_STUB_EFFECTIVE_ROLE"
	envPGStubAttributes = "TW_API_PROXY_POSTS_STUB_ATTRIBUTES"
)

// errPGStubInProduction stops the stub handing every teacher the same identity.
// Per request, not at startup: refusing at boot means New returning an error.
var errPGStubInProduction = errors.New(
	"PG identity stub is compiled in but TW_ENV is production: delete " +
		"proxy_pg_identity_stub.go and read the identity from the session")

// pgIdentityFor returns the identity to stamp into the PG token. It takes the
// request so the signature already matches a session-backed implementation.
func pgIdentityFor(_ *http.Request, logger *slog.Logger, env config.Environment) (pgIdentity, error) {
	if env == config.EnvProduction {
		return pgIdentity{}, errPGStubInProduction
	}

	identity := pgIdentity{
		Subject:       strings.TrimSpace(os.Getenv(envPGStubSubject)),
		SchoolCode:    strings.TrimSpace(os.Getenv(envPGStubSchoolCode)),
		Roles:         pgStubRoles(),
		EffectiveRole: envOrDefault(envPGStubEffective, "TW_TEACHER"),
		Attributes:    splitList(os.Getenv(envPGStubAttributes), []string{"ATTR_PG_USER"}),
	}

	// Warned per request: PG's rejection otherwise reads like a bad signing key.
	var missing []string
	if identity.Subject == "" {
		missing = append(missing, envPGStubSubject)
	}
	if identity.SchoolCode == "" {
		missing = append(missing, envPGStubSchoolCode)
	}
	if len(missing) > 0 {
		logger.Warn("PG identity stub incomplete; PG will reject this request",
			"missing", strings.Join(missing, ", "),
			"hint", "set "+envPGStubSubject+" and "+envPGStubSchoolCode+" in .env",
		)
	}

	return identity, nil
}

// pgStubRoles reads the comma-separated role list, defaulting to TW_TEACHER.
func pgStubRoles() []string {
	return splitList(os.Getenv(envPGStubRoles), []string{"TW_TEACHER"})
}

// envOrDefault trims the named variable, falling back when it is empty.
func envOrDefault(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

// splitList parses a comma-separated list, falling back when it is empty.
func splitList(raw string, fallback []string) []string {
	var out []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
