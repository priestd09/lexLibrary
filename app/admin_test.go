// Copyright (c) 2017-2018 Townsourced Inc.

package app_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/lexLibrary/lexLibrary/app"
)

func TestAdmin(t *testing.T) {
	var admin *app.Admin

	reset := func(t *testing.T) {
		t.Helper()

		admin = resetAdmin(t, "admin", "adminpassword")

		err := admin.SetSetting("AllowPublicSignups", true)
		if err != nil {
			t.Fatalf("Error allowing public signups for testing: %s", err)
		}

	}

	t.Run("Overview", func(t *testing.T) {
		reset(t)

		overview, err := admin.Overview()
		if err != nil {
			t.Fatalf("Error getting admin overview: %s", err)
		}

		if overview == nil {
			t.Fatal("Admin Overview is nil")
		}
	})

	t.Run("InstanceUsers", func(t *testing.T) {
		reset(t)

		password := "passwordValue"

		inactive, err := app.UserNew("inactive", password)
		if err != nil {
			t.Fatal(err)
		}

		err = admin.SetUserActive(inactive.Username, false)
		if err != nil {
			t.Fatal(err)
		}

		loggedIn, err := app.UserNew("loggedin", password)
		if err != nil {
			t.Fatal(err)
		}

		_, err = loggedIn.NewSession(time.Now().Add(1*time.Hour), "", "")
		if err != nil {
			t.Fatal(err)
		}

		loggedOut, err := app.UserNew("loggedout", password)
		if err != nil {
			t.Fatal(err)
		}

		s, err := loggedOut.NewSession(time.Now().Add(1*time.Hour), "", "")
		if err != nil {
			t.Fatal(err)
		}
		err = s.Logout()
		if err != nil {
			t.Fatal(err)
		}

		multipleSessions, err := app.UserNew("multiplesessions", password)
		if err != nil {
			t.Fatal(err)
		}

		_, err = multipleSessions.NewSession(time.Now().Add(1*time.Hour), "", "")
		if err != nil {
			t.Fatal(err)
		}
		_, err = multipleSessions.NewSession(time.Now().Add(1*time.Hour), "", "")
		if err != nil {
			t.Fatal(err)
		}

		neverLoggedIn, err := app.UserNew("neverLoggedIn", password)
		if err != nil {
			t.Fatal(err)
		}

		err = loggedIn.SetName("John Doe", loggedIn.Version)
		if err != nil {
			t.Fatal(err)
		}

		err = loggedOut.SetName("James Doe", loggedOut.Version)
		if err != nil {
			t.Fatal(err)
		}

		tests := []struct {
			activeOnly bool
			loggedIn   bool
			search     string
			offset     int
			limit      int

			total  int
			result []*app.User
		}{
			{true, false, "", 0, 100, 5, []*app.User{admin.User(), loggedIn, loggedOut, multipleSessions,
				neverLoggedIn}},
			{false, true, "", 0, 100, 2, []*app.User{loggedIn, multipleSessions}},
			{true, true, "", 0, 100, 2, []*app.User{loggedIn, multipleSessions}},
			{false, false, "", 0, 100, 6, []*app.User{admin.User(), loggedIn, loggedOut, multipleSessions,
				inactive, neverLoggedIn}},
			{false, false, "", 0, 2, 6, []*app.User{neverLoggedIn, multipleSessions}},
			{false, false, "", 2, 2, 6, []*app.User{loggedOut, loggedIn}},
			{false, false, "", 4, 2, 6, []*app.User{inactive, admin.User()}},
			{false, false, "John", 0, 100, 1, []*app.User{loggedIn}},
			{false, false, "logged", 0, 100, 3, []*app.User{loggedIn, loggedOut, neverLoggedIn}},
			{false, false, "Doe", 0, 100, 2, []*app.User{loggedIn, loggedOut}},
			{false, true, "DOE", 0, 100, 1, []*app.User{loggedIn}},
		}

		for i, test := range tests {
			t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
				users, total, err := admin.InstanceUsers(test.activeOnly, test.loggedIn, test.search,
					test.offset, test.limit)
				if err != nil {
					t.Fatal(err)
				}

				if total != test.total {
					t.Fatalf("Total doesn't match. Expected %d, got %d", test.total, total)
				}

				if len(users) != len(test.result) {
					t.Fatalf("Result len doesn't match. Expected %d, got %d", len(test.result),
						len(users))
				}

				for _, result := range test.result {
					found := false
					for _, user := range users {
						if user.Username == result.Username {
							found = true
							break
						}
					}
					if !found {
						t.Fatalf("User %s was not found in the result set.", result.Username)
					}
				}
			})
		}

	})

	t.Run("InstanceUser", func(t *testing.T) {
		reset(t)

		user, err := app.UserNew("instanceUserTest", "testInstanceUserPassword")
		if err != nil {
			t.Fatal(err)
		}

		iu, err := admin.InstanceUser("instanceUSERTEST")
		if err != nil {
			t.Fatal(err)
		}

		if iu.Username != user.Username || iu.ID != user.ID {
			t.Fatalf("Incorrect user returned. Expected %s Got %s", user.ID, iu.ID)
		}

		if iu.LastLogin.Valid {
			t.Fatalf("Instance user's last login was valid. Expected %v got %v", false, iu.LastLogin.Valid)
		}

		_, err = user.NewSession(time.Now().Add(1*time.Hour), "", "")
		if err != nil {
			t.Fatal(err)
		}

		iu, err = admin.InstanceUser("instanceusertest")
		if err != nil {
			t.Fatal(err)
		}
		if iu.Username != user.Username || iu.ID != user.ID {
			t.Fatalf("Incorrect user returned. Expected %s Got %s", user.ID, iu.ID)
		}

		if !iu.LastLogin.Valid {
			t.Fatalf("Instance user's last login was not valid. Expected %v got %v", true,
				iu.LastLogin.Valid)
		}

	})
}