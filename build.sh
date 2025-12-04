#!/bin/bash

mkdir -p dist
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsc_darwin_amd64 ./cmd/grsc
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsc_darwin_arm64 ./cmd/grsc
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsc_linux_amd64 ./cmd/grsc
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsc_linux_arm64 ./cmd/grsc
CGO_ENABLED=0 GOOS=linux GOARCH=mips go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsc_linux_mips ./cmd/grsc
CGO_ENABLED=0 GOOS=linux GOARCH=arm go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsc_linux_arm ./cmd/grsc
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsc_windows.exe ./cmd/grsc

CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsu_darwin_amd64 ./cmd/grsu
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsu_darwin_arm64 ./cmd/grsu
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsu_linux_amd64 ./cmd/grsu
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsu_linux_arm64 ./cmd/grsu
CGO_ENABLED=0 GOOS=linux GOARCH=mips go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsu_linux_mips ./cmd/grsu
CGO_ENABLED=0 GOOS=linux GOARCH=arm go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsu_linux_arm ./cmd/grsu
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -trimpath -ldflags "-s -w" -o ./cmd/grss/client/grsu_windows.exe ./cmd/grsu

go-bindata  -nomemcopy -nometadata -prefix cmd/grss/client -o ./cmd/grss/files.go ./cmd/grss/client/

CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -tags forceposix -trimpath -ldflags "-s -w" -o ./dist/grss_darwin_amd64 ./cmd/grss
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags forceposix -trimpath -ldflags "-s -w" -o ./dist/grss_darwin_arm64 ./cmd/grss
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags forceposix -trimpath -ldflags "-s -w" -o ./dist/grss_linux_amd64 ./cmd/grss
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags forceposix -trimpath -ldflags "-s -w" -o ./dist/grss_linux_arm64 ./cmd/grss
CGO_ENABLED=0 GOOS=linux GOARCH=mips go build -tags forceposix -trimpath -ldflags "-s -w" -o ./dist/grss_linux_mips ./cmd/grss
CGO_ENABLED=0 GOOS=linux GOARCH=arm go build -tags forceposix -trimpath -ldflags "-s -w" -o ./dist/grss_linux_arm ./cmd/grss
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -tags forceposix -trimpath -ldflags "-s -w" -o ./dist/grss_windows.exe ./cmd/grss

cp README.md ./dist
cp README-REALITY.md ./dist