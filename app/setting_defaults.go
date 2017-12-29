// Copyright (c) 2017 Townsourced Inc.

package app

// settingDefaults are the default settings that Lex Library starts with.  If a setting is not overridden in the database
// then the default values here are used
var settingDefaults = []Setting{
	Setting{
		ID:          "AllowPublic",
		Category:    "Documents",
		Description: "Whether or not to allow documents to be published that are accessible without logging in to Lex Library",
		Value:       true,
	},
	Setting{
		ID:          "AuthenticationType",
		Category:    "Users",
		Description: "How users log into Lex Library.", // TODO add description of auth options into setting description
		Value:       AuthTypePassword,
		Options: []interface{}{
			AuthTypePassword,
		},
	},
	Setting{
		ID:          "PasswordMinLength",
		Category:    "Users",
		Description: "Required minimum length for passwords",
		Value:       10,
		validate: func(value interface{}) error {
			if value.(int) < 8 {
				return NewFailure("Minimum password length must be greater than 8")
			}
			return nil
		},
	},
}
