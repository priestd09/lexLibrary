#!/bin/bash
set -e

echo Running Tests against $LLDATABASE

cd ..
./build.sh
go test ./data -config $PWD/ci/$LLDATABASE/config.yaml
go test ./app -config $PWD/ci/$LLDATABASE/config.yaml
# go test ./web -config $PWD/ci/$LLDATABASE/config.yaml

#TODO: gulp test - frontend tests