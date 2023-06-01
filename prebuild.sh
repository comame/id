#! /bin/bash

pushd front
    npm ci
    npm run build
popd

cargo build -r

pushd dashboard
    pushd web
        npm ci
        npm run build
    popd
    go build -o out
popd
