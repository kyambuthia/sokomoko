package db

import "testing"

func TestListUsersByRolesPagesAndSearches(t *testing.T) {
	s := newTestStore(t)
	for _, name := range []string{"ann", "bob", "cara", "dan_x", "danx"} {
		if _, err := s.CreateUser(t.Context(), User{Username: name, Email: name + "@example.com", PasswordHash: "hash", Role: RoleUser}); err != nil {
			t.Fatal(err)
		}
	}
	staff := mustCreateUser(t, s, RoleStaff)
	gone := mustCreateUser(t, s, RoleUser)
	if err := s.DeleteUser(t.Context(), gone); err != nil {
		t.Fatal(err)
	}

	page, total, err := s.ListUsersByRoles(t.Context(), UserSearch{Roles: []string{RoleUser}, Limit: 2, Offset: 2})
	if err != nil || total != 5 || len(page) != 2 || page[0].Username != "cara" || page[1].Username != "dan_x" {
		t.Fatalf("second page = %v total=%d err=%v", usernames(page), total, err)
	}

	// The underscore is literal, not a LIKE wildcard.
	found, total, err := s.ListUsersByRoles(t.Context(), UserSearch{Roles: []string{RoleUser}, Query: "dan_", Limit: 10})
	if err != nil || total != 1 || len(found) != 1 || found[0].Username != "dan_x" {
		t.Fatalf("search = %v total=%d err=%v", usernames(found), total, err)
	}

	// Email matches too, and roles outside the search are left out.
	found, total, err = s.ListUsersByRoles(t.Context(), UserSearch{Roles: []string{RoleUser, RoleStaff}, Query: "BOB@EXAMPLE", Limit: 10})
	if err != nil || total != 1 || found[0].Username != "bob" {
		t.Fatalf("email search = %v total=%d err=%v", usernames(found), total, err)
	}
	all, total, err := s.ListUsersByRoles(t.Context(), UserSearch{Roles: []string{RoleUser, RoleStaff}, Limit: 50})
	if err != nil || total != 6 || all[0].ID != staff { // ordered by role: staff before user
		t.Fatalf("all = %v total=%d err=%v", usernames(all), total, err)
	}
}

func usernames(users []User) []string {
	names := make([]string, len(users))
	for i, u := range users {
		names[i] = u.Username
	}
	return names
}
