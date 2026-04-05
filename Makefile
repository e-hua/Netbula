.DEFAULT_GOAL := build
# Defines which target is run when no target is specified (i.e. When running `make` alone)

.PHONY:fmt vet test build report 

fmt: 
	go fmt ./...

# The target 'vet' depends on target 'fmt'
vet: fmt 
	go vet ./...

test: vet
	go test -v -cover -coverprofile=c.out ./...

build: test 
	go build -o bin/netbula main.go

report: test 
	go tool cover -html=c.out