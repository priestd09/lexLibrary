package app

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lexLibrary/lexLibrary/data"
	"golang.org/x/sync/errgroup"
)

// Admin is a wrapper around User that only provides access to admin level functions
type Admin struct {
	user *User
}

// ErrNotAdmin is returned when an admin activity is attempted by a non-admin user
var ErrNotAdmin = Unauthorized("This functionality is reserved for administrators only")

// The methods defined here should mostly be wrappers around the actual code doing the inserts / updates / deletes
// The idea is you can come to the admin source file to see all of the funcitonality an admin can perform, but to see
// the detail of what is actually being performed you should goto the proper source file for the functionality: settings
// users, etc

var sqlAdmin = struct {
	stats,
	user,
	init *data.Query
	users func(bool, bool, bool, bool) *data.Query
}{
	stats: data.NewQuery(`
		select users.num, sessions.num, documents.num, errorsTotal.num, errorsSinceStart.num
		from
		(select count(*) as num from users where active = {{TRUE}}) as users,
		(
			select count(*) as num from (
				select count(*) as num from sessions 
				where expires > {{NOW}} and valid = {{TRUE}} 
				group by user_id
			) as user_sessions
		) as sessions,
		(select 0 as num) as documents, 
		(select count(*) num from logs) as errorsTotal,
		(select count(*) num 
			from logs where occurred >= {{arg "start"}}
		) as errorsSinceStart
	`),
	init: data.NewQuery(`
		select occurred from schema_versions where version = 0
	`),
	user: data.NewQuery(fmt.Sprintf(`
		select 	%s, s.created
		from 	users u
			left outer join sessions s
				on u.id = s.user_id
		where 	(s.created = (
			select 	max(s2.created)
			from 	sessions s2
			where s2.user_id = s.user_id
		) or s.created is null)
		and u.username = {{arg "username"}}
	`, userPublicColumns)),
	users: func(active, loggedIn, search, total bool) *data.Query {
		columns := userPublicColumns + ", s.created"
		if total {
			columns = "count(*)"
		}

		criteria := ""
		if loggedIn {
			criteria = "and s.expires > {{NOW}} and s.valid = {{TRUE}} "
		}
		if active {
			criteria += "and u.active = {{TRUE}} "
		}

		if search {
			criteria += `and (
				username like {{arg "usernameSearch"}}
				or lower(name) like {{arg "nameSearch"}} 	
			)`
		}

		// NOTE: CockroachDB doesn't support sub queries like this yet:
		// https://github.com/cockroachdb/cockroach/issues/3288
		qry := fmt.Sprintf(`
			select 	%s
			from 	users u
				left outer join sessions s
					on u.id = s.user_id
			where 	(s.created = (
				select 	max(s2.created)
				from 	sessions s2
				where s2.user_id = s.user_id
			) or s.created is null)
			%s
		`, columns, criteria)
		if !total {
			qry += `
			order by u.created desc
			{{if sqlserver}}
				OFFSET {{arg "offset"}} ROWS FETCH NEXT {{arg "limit"}} ROWS ONLY
			{{else}}
				LIMIT {{arg "limit" }} OFFSET {{arg "offset"}}
			{{end}}
		`
		}
		return data.NewQuery(qry)
	},
}

// Setting will look for a setting that has the passed in id
func (a *Admin) Setting(id string) (Setting, error) {
	return settingGet(id)
}

// User returns the underlying user for the admin
func (a *Admin) User() *User {
	return a.user
}

// SetSetting updates a settings value
func (a *Admin) SetSetting(id string, value interface{}) error {
	return settingSet(nil, id, value)
}

// SetMultipleSettings sets multiple settings in the same transaction
func (a *Admin) SetMultipleSettings(settings map[string]interface{}) error {
	return settingSetMultiple(settings)
}

// SetUserActive sets the active status of the given user
func (a *Admin) SetUserActive(username string, active bool) error {
	u, err := userFromUsername(nil, username)
	if err != nil {
		return err
	}

	return u.setActive(active, u.Version)
}

// SetUserAdmin sets if a user is an Administrator or not
func (a *Admin) SetUserAdmin(username string, admin bool) error {
	u, err := userFromUsername(nil, username)
	if err != nil {
		return err
	}

	return u.setAdmin(admin, u.Version)
}

// Overview is a collection of statistics and information about the LL instance
type Overview struct {
	Instance struct {
		Users     int
		Documents int
		Sessions  int
		Size      struct {
			data.SizeStats
		}
		Uptime           time.Duration
		FirstLaunch      time.Time
		Version          string
		BuildDate        time.Time
		ErrorsTotal      int
		ErrorsSinceStart int
	}
	System  sysInfo
	Runtime RuntimeInfo

	data.Config
}

// Overview returns statistics on the current instance
func (a *Admin) Overview() (*Overview, error) {
	o := &Overview{
		Config: data.CurrentCFG(),
	}
	o.Config.DatabaseURL = "" // hide to prevent showing db password

	// Instance Stats
	err := sqlAdmin.stats.QueryRow(data.Arg("start", initTime)).Scan(
		&o.Instance.Users,
		&o.Instance.Sessions,
		&o.Instance.Documents,
		&o.Instance.ErrorsTotal,
		&o.Instance.ErrorsSinceStart,
	)
	if err != nil {
		return nil, err
	}

	o.Instance.Uptime = time.Since(initTime)

	var firstLaunch time.Time
	err = sqlAdmin.init.QueryRow().Scan(&firstLaunch)
	if err != nil {
		return nil, err
	}
	o.Instance.FirstLaunch = firstLaunch

	o.Instance.Version = Version()
	o.Instance.BuildDate = BuildDate()

	// Size Stats
	size, err := data.Size()
	if err != nil {
		return nil, err
	}

	o.Instance.Size.SizeStats = size

	o.Runtime = runtimeInfo
	o.System = systemInfo()

	return o, nil
}

type InstanceUser struct {
	PublicProfile
	LastLogin data.NullTime
}

func (u *InstanceUser) scan(record scanner) error {
	err := record.Scan(
		&u.ID,
		&u.Username,
		&u.Name,
		&u.Active,
		&u.profileImage,
		&u.admin,
		&u.Created,
		&u.LastLogin,
	)
	if err == sql.ErrNoRows {
		return ErrUserNotFound
	}
	return err
}

// InstanceUsers returns a list of all of the current users in Lex Library
func (a *Admin) InstanceUsers(activeOnly, loggedIn bool, search string, offset, limit int) (
	users []*InstanceUser, total int, err error) {
	if limit == 0 || limit > maxRows {
		limit = 10
	}

	runSearch := search != ""

	search = "%" + search + "%"

	var g errgroup.Group

	g.Go(func() error {
		rows, err := sqlAdmin.users(activeOnly, loggedIn, runSearch, false).Query(
			data.Arg("limit", limit),
			data.Arg("offset", offset),
			data.Arg("usernameSearch", search),
			data.Arg("nameSearch", search),
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			u := &InstanceUser{}
			err = u.scan(rows)
			if err != nil {
				return err
			}
			users = append(users, u)
		}
		return nil
	})

	g.Go(func() error {
		return sqlAdmin.users(activeOnly, loggedIn, search != "", true).QueryRow(
			data.Arg("usernameSearch", search),
			data.Arg("nameSearch", search),
		).Scan(&total)
	})

	err = g.Wait()
	if err != nil {
		return nil, total, err
	}

	return users, total, nil
}

// InstanceUser returns an instance level view of a given user
func (a *Admin) InstanceUser(username string) (*InstanceUser, error) {
	u := &InstanceUser{}
	err := u.scan(sqlAdmin.user.QueryRow(data.Arg("username", username)))
	if err != nil {
		return nil, err
	}

	return u, nil
}
