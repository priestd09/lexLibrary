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
}
