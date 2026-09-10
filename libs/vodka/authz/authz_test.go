package authz

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"dsc/libs/vodka"

	"github.com/casbin/casbin"
)

func testRequest(t *testing.T, ce *casbin.Enforcer, user string, path string, method string, code int) {
	e := vodka.New()
	req := httptest.NewRequest(method, path, nil)
	req.SetBasicAuth(user, "secret")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res, func(c *vodka.Context) error {
		return c.String("test", vodka.StatusOK)
	})

	h := Auth(ce)

	err := h(c)

	if err != nil {
		if errObj, ok := err.(*vodka.HTTPError); ok {
			if errObj.StatusCode() != code {
				t.Errorf("%s, %s, %s: %d, supposed to be %d", user, path, method, errObj.StatusCode(), code)
			}
		} else {
			t.Error(err)
		}
	} else {
		if c.Response.Status != code {
			t.Errorf("%s, %s, %s: %d, supposed to be %d", user, path, method, c.Response.Status, code)
		}
	}
}

func TestAuth(t *testing.T) {
	ce := NewEnforcer("auth_model.conf", "auth_policy.csv")

	fmt.Println("GetPermissionsForUser>", ce.GetPermissionsForUser("alice"))
	s, e := ce.GetRolesForUser("alice")
	fmt.Printf("GetRolesForUser> %v %v\n", s, e)
	fmt.Println("GetAllRoles>", ce.GetAllRoles())
	fmt.Println("GetAllSubjects>", ce.GetAllSubjects())
	fmt.Println("GetAllActions>", ce.GetAllActions())
	fmt.Println("GetAllObjects>", ce.GetAllObjects())

	testRequest(t, ce, "alice", "/dataset1/resource1", vodka.GET, 200)
	testRequest(t, ce, "alice", "/dataset1/resource1", vodka.POST, 200)
	testRequest(t, ce, "alice", "/dataset1/resource2", vodka.GET, 200)
	testRequest(t, ce, "alice", "/dataset1/resource2", vodka.POST, 403)
}

func TestPathWildcard(t *testing.T) {
	ce, err := NewEnforcerSafe("auth_model.conf", "auth_policy.csv")
	if err != nil {
		panic(err)
	}

	fmt.Println("GetPermissionsForUser>", ce.GetPermissionsForUser("bob"))
	s, e := ce.GetRolesForUser("bob")
	fmt.Printf("GetRolesForUser> %v %v\n", s, e)
	fmt.Println("GetAllRoles>", ce.GetAllRoles())
	fmt.Println("GetAllSubjects>", ce.GetAllSubjects())
	fmt.Println("GetAllActions>", ce.GetAllActions())
	fmt.Println("GetAllObjects>", ce.GetAllObjects())

	testRequest(t, ce, "bob", "/dataset2/resource1", "GET", 200)
	testRequest(t, ce, "bob", "/dataset2/resource1", "POST", 200)
	testRequest(t, ce, "bob", "/dataset2/resource1", "DELETE", 200)
	testRequest(t, ce, "bob", "/dataset2/resource2", "GET", 200)
	testRequest(t, ce, "bob", "/dataset2/resource2", "POST", 403)
	testRequest(t, ce, "bob", "/dataset2/resource2", "DELETE", 403)

	testRequest(t, ce, "bob", "/dataset2/folder1/item1", "GET", 403)
	testRequest(t, ce, "bob", "/dataset2/folder1/item1", "POST", 200)
	testRequest(t, ce, "bob", "/dataset2/folder1/item1", "DELETE", 403)
	testRequest(t, ce, "bob", "/dataset2/folder1/item2", "GET", 403)
	testRequest(t, ce, "bob", "/dataset2/folder1/item2", "POST", 200)
	testRequest(t, ce, "bob", "/dataset2/folder1/item2", "DELETE", 403)
}

func TestRBAC(t *testing.T) {
	ce := NewEnforcer("auth_model.conf", "auth_policy.csv")

	// cathy can access all /dataset1/* resources via all methods because it has the dataset1_admin role.
	testRequest(t, ce, "cathy", "/dataset1/item", "GET", 200)
	testRequest(t, ce, "cathy", "/dataset1/item", "POST", 200)
	testRequest(t, ce, "cathy", "/dataset1/item", "DELETE", 200)
	testRequest(t, ce, "cathy", "/dataset2/item", "GET", 403)
	testRequest(t, ce, "cathy", "/dataset2/item", "POST", 403)
	testRequest(t, ce, "cathy", "/dataset2/item", "DELETE", 403)

	fmt.Println("GetPermissionsForUser>", ce.GetPermissionsForUser("cathy"))
	s, e := ce.GetRolesForUser("cathy")
	fmt.Printf("GetRolesForUser> %v %v\n", s, e)
	fmt.Println("GetAllRoles>", ce.GetAllRoles())
	fmt.Println("GetAllSubjects>", ce.GetAllSubjects())
	fmt.Println("GetAllActions>", ce.GetAllActions())
	fmt.Println("GetAllObjects>", ce.GetAllObjects())

	// delete all roles on user cathy, so cathy cannot access any resources now.
	ce.DeleteRolesForUser("cathy")

	testRequest(t, ce, "cathy", "/dataset1/item", "GET", 403)
	testRequest(t, ce, "cathy", "/dataset1/item", "POST", 403)
	testRequest(t, ce, "cathy", "/dataset1/item", "DELETE", 403)
	testRequest(t, ce, "cathy", "/dataset2/item", "GET", 403)
	testRequest(t, ce, "cathy", "/dataset2/item", "POST", 403)
	testRequest(t, ce, "cathy", "/dataset2/item", "DELETE", 403)
}
