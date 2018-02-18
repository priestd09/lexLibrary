// Copyright (c) 2017-2018 Townsourced Inc.

package web

import (
	"net/http"

	"github.com/lexLibrary/lexLibrary/app"
)

type settingInput struct {
	ID       *string
	Value    interface{}
	Settings map[string]interface{}
}

func settingPut(w http.ResponseWriter, r *http.Request, c ctx) {
	if c.session == nil {
		unauthorized(w, r)
		return
	}

	input := &settingInput{}

	if errHandled(parseInput(r, input), w, r) {
		return
	}

	u, err := c.session.User()
	if errHandled(err, w, r) {
		return
	}

	if input.ID == nil {
		if input.Settings == nil {
			errHandled(app.NewFailure("ID must be set"), w, r)
			return
		}

		if errHandled(app.SettingSetMultiple(u, input.Settings), w, r) {
			return
		}

		respond(w, success(nil))
		return
	}

	if errHandled(app.SettingSet(u, *input.ID, input.Value), w, r) {
		return
	}

	respond(w, success(nil))
}

// settingDelete sets a setting to it's default value
func settingDelete(w http.ResponseWriter, r *http.Request, c ctx) {
	if c.session == nil {
		unauthorized(w, r)
		return
	}
	input := &settingInput{}

	if errHandled(parseInput(r, input), w, r) {
		return
	}

	if input.ID == nil {
		errHandled(app.NewFailure("ID must be set"), w, r)
		return
	}
	u, err := c.session.User()
	if errHandled(err, w, r) {
		return
	}

	setting, err := app.SettingDefault(*input.ID)
	if errHandled(err, w, r) {
		return
	}

	if errHandled(app.SettingSet(u, *input.ID, setting.Value), w, r) {
		return
	}

	respond(w, success(nil))
}