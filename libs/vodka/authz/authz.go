/* Package casbin provides middleware to enable ACL, RBAC, ABAC authorization support.

Simple example:

	package main

	import (
		"dsc/libs/vodka"
		"dsc/libs/vodka/authz"
	)

	func main() {
		e := vodka.New()

		// Mediate the access for every request
		e.Use(authz.Auth(authz.NewEnforcer("auth_model.conf", "auth_policy.csv")))

		e.Logger.Fatal(e.Start(":1323"))
	}

Advanced example:

	package main

	import (
		"dsc/libs/vodka"
		"dsc/libs/vodka/authz"
	)

	func main() {
		ce := authz.NewEnforcer("auth_model.conf", "")
		ce.AddRoleForUser("alice", "admin")
		ce.AddPolicy(...)

		e := vodka.New()

		vodka.Use(authz.Auth(ce))

		e.Logger.Fatal(e.Start(":1323"))
	}
*/

package authz

import (
	"dsc/libs/vodka"
	"dsc/libs/vodka/skipper"

	"github.com/casbin/casbin"
)

type (
	// AuthConfig defines the config for CasbinAuth middleware.
	AuthConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper skipper.Skipper
		// Enforcer CasbinAuth main rule.
		// Required.
		Enforcer *casbin.Enforcer
	}
)

var (
	// DefaultAuthConfig is the default CasbinAuth middleware config.
	DefaultAuthConfig = AuthConfig{
		Skipper: skipper.DefaultSkipper,
	}
)

// NewEnforcer gets an enforcer via CONF, file or DB.
func NewEnforcer(params ...interface{}) *casbin.Enforcer {
	return casbin.NewEnforcer(params...)
}

// NewEnforcerSafe calls NewEnforcer in a safe way, returns error instead of causing panic.
func NewEnforcerSafe(params ...interface{}) (*casbin.Enforcer, error) {
	return casbin.NewEnforcerSafe(params...)
}

// Auth returns an Auth middleware.
//
// For valid credentials it calls the next handler.
// For missing or invalid credentials, it sends "401 - Unauthorized" response.
func Auth(ce *casbin.Enforcer) vodka.Handler {
	c := DefaultAuthConfig
	c.Enforcer = ce
	return AuthWithConfig(c)
}

// AuthWithConfig returns a CasbinAuth middleware with config.
// See `Auth()`.
func AuthWithConfig(config AuthConfig) vodka.Handler {
	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultAuthConfig.Skipper
	}

	return func(c *vodka.Context) error {
		if config.Skipper(c) || config.CheckPermission(c) {
			return c.Next()
		}

		return vodka.ErrForbidden
	}
}

// GetUserName gets the user name from the request.
// Currently, only HTTP basic authentication is supported
func (a *AuthConfig) GetUserName(c *vodka.Context) string {
	username, _, _ := c.Request.BasicAuth()
	return username
}

// CheckPermission checks the user/method/path combination from the request.
// Returns true (permission granted) or false (permission forbidden)
func (a *AuthConfig) CheckPermission(c *vodka.Context) bool {
	user := a.GetUserName(c)
	method := c.Request.Method
	path := c.Request.URL.Path
	return a.Enforcer.Enforce(user, path, method)
}
