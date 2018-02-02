// Copyright (c) 2017-2018 Townsourced Inc.
import * as xhr from './lib/xhr';
import './lib/polyfill';

var vm = new Vue({
    el: '#setup',
    data: function() {
        return {
            username: '',
            password: '',
            password2: '',
            error: null,
            password2Err: null,
            showSettings: true,
        };
    },
    directives: {
        focus: {
            inserted: function(el) {
                el.focus();
            },
        },
    },
    methods: {
        signup: function(e) {
            e.preventDefault();
            this.error = null;
            if (this.password2Err) {
                return;
            }
            if (!this.username) {
                this.error = 'A username is required';
                return;
            }
            if (!this.password) {
                this.error = 'A password is required';
                return;
            }
            if (this.password !== this.password2) {
                this.password2Err = 'Passwords do not match';
                return;
            }

            xhr.post('/user', {
                    username: this.username,
                    password: this.password,
                })
                .then((result) => {
                    this.showSettings = true;
                })
                .catch((err) => {
                    this.error = err.content;
                });
        },
        validatePassword2: function() {
            if (this.password2Err) {
                return;
            }
            if (!this.password2) {
                return;
            }
            if (this.password !== this.password2) {
                this.password2Err = 'Passwords do not match';
            }
        },
    },
});
