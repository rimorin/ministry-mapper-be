// Package authz holds what every package must agree on about roles.
package authz

import "strings"

// StaffRoles are the roles that open a congregation's data. A publisher holds
// a role row too but only works the links given to them, so "has a role" in an
// access check always means one of these: a role added later stays shut out
// until it is listed here.
var StaffRoles = []string{"administrator", "conductor", "read_only"}

// StaffRoleSQL is StaffRoles as a SQL list, for queries that join roles.
var StaffRoleSQL = "('" + strings.Join(StaffRoles, "', '") + "')"
