// Copyright (c) 2017-2018 Townsourced Inc.

package data

/*
	Array index determines the schema version
	Add new schema versions to the bottom of the array
	Once you push your changes to the main git repository, add new schema versions for
	table changes, rather than updating existing ones

	Stick to the following column types:

	+------------------------------+
	|go        | sql type          |
	|----------|-------------------|
	|nil       | null              |
	|int       | integer           |
	|int64     | integer           |
	|float64   | float             |
	|bool      | integer           |
	|[]byte    | blob              |
	|string    | text              |
	|string    | nvarchar(size)    |
	|time.Time | timestamp/datetime|
	+------------------------------+


	Keep column and table names in lowercase and separate words with underscores
	tables should be named for their collections (i.e. plural)

	For best compatibility, only have one statement per query; i.e. no semicolons, and don't use any reserved words

	String / Text types will be by default case sensitive and unicode supported. The default database collations should
	reflect that.  Prefer Text over varchar except where necessary such as PKs.

	DateTime types are only precise up to Milliseconds

	Integers are 64 bit

	Add new versions for changes to exising tables if the changes have been checked into the Dev or master branches

	When in doubt, define the database tables to match the behavior of Go.  For instance, no null / nil types on
	strings, datetimes, booleans, etc.  Strings should be case sensitive, because they are in Go.  An unset time
	is it's Zero value which should be the column default.
*/

var schemaVersions = []*Query{
	NewQuery(`
		create table schema_versions (
			version INTEGER PRIMARY KEY NOT NULL,
			script {{text}} NOT NULL
		)
	`),
	NewQuery(`
		create table logs (
			id {{id}} PRIMARY KEY NOT NULL,
			occurred {{datetime}} NOT NULL, 
			message {{text}} NOT NULL
		)
	`),
	NewQuery(`
		create index i_occurred on logs (occurred)
	`),
	NewQuery(`
		create table settings (
			id {{varchar 64}} PRIMARY KEY NOT NULL,
			description {{text}} NOT NULL,
			value {{text}}  NOT NULL
		)
	`),
	NewQuery(`
		create table users (
			id {{id}} PRIMARY KEY NOT NULL,
			username {{varchar 64}} NOT NULL,
			name {{text}},
			auth_type {{text}} NOT NULL,
			password {{bytes}},
			password_version {{int}},
			password_expiration {{datetime}},
			active {{bool}},
			admin {{bool}} NOT NULL,
			version {{int}} NOT NULL,
			updated {{datetime}} NOT NULL,
			created {{datetime}} NOT NULL
		)
	`),
	NewQuery(`
		create table sessions (
			id {{varchar 32}} NOT NULL,
			user_id {{id}} NOT NULL REFERENCES users(id),
			valid {{bool}} NOT NULL,
			expires {{datetime}} NOT NULL,
			ip_address {{text}} NOT NULL,
			user_agent {{text}},
			csrf_token {{text}} NOT NULL,
			csrf_date {{datetime}} NOT NULL,
			updated {{datetime}} NOT NULL,
			created {{datetime}} NOT NULL,
			PRIMARY KEY(id, user_id)
		)
	`),
	NewQuery(`
		create index i_username on users (username)
	`),
	NewQuery(`
		create table images (
			id {{id}} PRIMARY KEY NOT NULL,
			name {{text}} NOT NULL,
			version {{int}} NOT NULL,
			content_type {{text}} NOT NULL,
			data {{bytes}} NOT NULL,
			thumb {{bytes}} NOT NULL,
			placeholder {{bytes}} NOT NULL,
			updated {{datetime}} NOT NULL,
			created {{datetime}} NOT NULL
		)	
	`),
	NewQuery(`
		{{if cockroachdb}}
			alter table users add column profile_image_id {{id}};
			CREATE INDEX ON users (profile_image_id);
			alter table users add foreign key (profile_image_id) references images(id);
		{{else}}
			alter table users add profile_image_id {{id}} REFERENCES images(id)
		{{end}}
	`),
	NewQuery(`
		{{if cockroachdb}}
			alter table users add column profile_image_draft_id {{id}};
			CREATE INDEX ON users (profile_image_draft_id);
			alter table users add foreign key (profile_image_draft_id) references images(id);
		{{else}}
			alter table users add profile_image_draft_id {{id}} REFERENCES images(id)
		{{end}}
	`),
	NewQuery(`
		create table groups (
			id {{id}} PRIMARY KEY NOT NULL,
			name {{text}} UNIQUE NOT NULL,
			version {{int}} NOT NULL,
			updated {{datetime}} NOT NULL,
			created {{datetime}} NOT NULL
		)	
	`),
	NewQuery(`
		create table users_to_groups (
			user_id {{id}} NOT NULL REFERENCES users(id),
			group_id {{id}} NOT NULL REFERENCES groups(id),
			admin {{bool}},
			PRIMARY KEY(user_id, group_id)
		)
	`),
	NewQuery(`
		create index i_name on groups (name)
	`),
}
