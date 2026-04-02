.DEFAULT_GOAL := build
# Defines which target is run when no target is specified (i.e. When running `make` alone)

.PHONY:fmt vet build

fmt: 
	go fmt ./...

# The target 'vet' depends on target 'fmt'
vet: fmt 
	go vet ./...

build: vet 
	go build -o bin/netbula main.go
