# Lex Library
Lex Library is an open-source documentation repository with modern features, like importing from a wide variety of documentation files and web sites, and natural language processing for automatic tagging and summaries.


# Building

**Requirements**:
 * Go 1.9 or greater
 * Node 8 or greater
 * go-binddata - `go get -u github.com/shuLhan/go-bindata/...`


`run.sh` - build LL, build the client files, and run LL in dev mode where web files are rebuilt and loaded on the fly, and templates
    are rebuilt on each request.

`build.sh` - build LL, build the client files, and embed the static files into the binary, made for release.  Increments
    the semver in the verison file which is also embedded into the binary and used for things like e-tagging static
    assets.