package auth

import "github.com/carloseduardo21/apisix-dashboard/internal/i18n"

// Roles, from least to most privileged (PCI DSS 7.2: least privilege).
const (
	RoleViewer   = "viewer"   // read-only access to APISIX configuration
	RoleOperator = "operator" // viewer + changes to APISIX configuration
	RoleAdmin    = "admin"    // operator + dashboard users and audit trail
)

func ValidRole(r string) bool {
	return r == RoleViewer || r == RoleOperator || r == RoleAdmin
}

func CanWrite(role string) bool { return role == RoleOperator || role == RoleAdmin }
func IsAdmin(role string) bool  { return role == RoleAdmin }

// ValidationError is an error caused by user input; its message is safe to
// show in the UI.
type ValidationError struct{ msg i18n.Message }

func (e *ValidationError) Error() string             { return e.msg.String() }
func (e *ValidationError) UserMessage() i18n.Message { return e.msg }

// validationErr builds a ValidationError (see i18n.M for the arguments).
func validationErr(code, template string, kv ...any) error {
	return &ValidationError{msg: i18n.M(code, template, kv...)}
}
