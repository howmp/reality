#!/bin/bash

mkdir -p dist
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsc_darwin ./cmd/grsc
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsc_linux ./cmd/grsc
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsc_windows.exe ./cmd/grsc

CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsu_darwin ./cmd/grsu
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsu_linux ./cmd/grsu
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsu_windows.exe ./cmd/grsu

go-bindata  -nomemcopy -nometadata -prefix cmd/grss/client -o ./cmd/grss/files.go ./cmd/grss/client/

CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -tags forceposix -trimpath -ldflags "-s -w" -o ./dist/grss_darwin ./cmd/grss
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags forceposix -trimpath -ldflags "-s -w" -o ./dist/grss_linux ./cmd/grss
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -tags forceposix -trimpath -ldflags "-s -w" -o ./dist/grss_windows.exe ./cmd/grss

cp README.md ./dist
cp README-REALITY.md ./dist