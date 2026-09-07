package deltadiff

import (
	"encoding/json"
	"testing"
)

func TestNestedArrayAndObjectMutations(t *testing.T) {
	initialJSON := []byte(`{
		"users": [
			{"id": 1, "name": "Alice", "roles": ["viewer"]},
			{"id": 2, "name": "Bob", "roles": ["editor"]}
		],
		"config": {
			"database": {
				"host": "localhost",
				"port": 5432
			}
		}
	}`)

	patch := Patch{
		// 1. Test condition on Alice's name
		{Op: OpTest, Path: "/users/0/name", Value: "Alice"},
		// 2. Add role "admin" to Alice's roles
		{Op: OpAdd, Path: "/users/0/roles/1", Value: "admin"},
		// 3. Update DB port
		{Op: OpReplace, Path: "/config/database/port", Value: float64(5433)},
		// 4. Add new user Charlie using append (-)
		{Op: OpAdd, Path: "/users/-", Value: map[string]any{"id": float64(3), "name": "Charlie"}},
		// 5. Remove Bob's roles
		{Op: OpRemove, Path: "/users/1/roles"},
		// 6. Copy DB host to top-level primary_host
		{Op: OpCopy, Path: "/primary_host", From: "/config/database/host"},
	}

	resultBytes, err := ApplyJSON(initialJSON, patch)
	if err != nil {
		t.Fatalf("ApplyJSON failed: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal(resultBytes, &res); err != nil {
		t.Fatalf("unmarshal result failed: %v", err)
	}

	// Verify Alice roles
	users := res["users"].([]any)
	alice := users[0].(map[string]any)
	aliceRoles := alice["roles"].([]any)
	if len(aliceRoles) != 2 || aliceRoles[1] != "admin" {
		t.Fatalf("expected Alice to have [viewer, admin], got %v", aliceRoles)
	}

	// Verify Bob roles removed
	bob := users[1].(map[string]any)
	if _, exists := bob["roles"]; exists {
		t.Fatalf("expected Bob roles to be removed")
	}

	// Verify Charlie appended
	if len(users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(users))
	}
	charlie := users[2].(map[string]any)
	if charlie["name"] != "Charlie" {
		t.Fatalf("expected Charlie, got %v", charlie["name"])
	}

	// Verify port changed
	config := res["config"].(map[string]any)
	db := config["database"].(map[string]any)
	if db["port"] != float64(5433) {
		t.Fatalf("expected port 5433, got %v", db["port"])
	}

	// Verify copied host
	if res["primary_host"] != "localhost" {
		t.Fatalf("expected primary_host localhost, got %v", res["primary_host"])
	}
}
