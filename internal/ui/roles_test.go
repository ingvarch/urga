package ui

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// clientRoles are the roles Client is made of.
var clientRoles = []reflect.Type{
	reflect.TypeFor[clusterClient](),
	reflect.TypeFor[eventsClient](),
	reflect.TypeFor[jobsClient](),
	reflect.TypeFor[allocsClient](),
	reflect.TypeFor[deploymentsClient](),
	reflect.TypeFor[nodesClient](),
	reflect.TypeFor[namespacesClient](),
	reflect.TypeFor[servicesClient](),
	reflect.TypeFor[evaluationsClient](),
	reflect.TypeFor[variablesClient](),
	reflect.TypeFor[serversClient](),
	reflect.TypeFor[filesClient](),
}

// Every method of Client belongs to one role and only one: a method added to
// Client alone could not be required on its own, and one in two roles would
// blur what each is for.
func TestClient_EveryMethodHasOneRole(t *testing.T) {
	owner := map[string]string{}

	for _, role := range clientRoles {
		for i := range role.NumMethod() {
			name := role.Method(i).Name

			if other, ok := owner[name]; ok {
				t.Errorf("%s is in %s and in %s", name, other, role.Name())
			}

			owner[name] = role.Name()
		}
	}

	client := reflect.TypeFor[Client]()
	for i := range client.NumMethod() {
		if name := client.Method(i).Name; owner[name] == "" {
			t.Errorf("%s has no role", name)
		}
	}

	require.Len(t, owner, client.NumMethod(), "a role holds a method Client does not")
}
