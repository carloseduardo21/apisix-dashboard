package auth

import "fmt"

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
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

func validationErr(format string, args ...any) error {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}
