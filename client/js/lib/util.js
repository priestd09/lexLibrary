// Copyright (c) 2017-2018 Townsourced Inc.
export

function payload(id = 'payload') {
    let el = document.getElementById(id);

    if (!el) {
        return null;
    }

    if (!el.innerHTML) {
        return null;
    }

    if (el.innerHTML.trim() === "") {
        return null;
    }
    return JSON.parse(el.innerHTML);
}
