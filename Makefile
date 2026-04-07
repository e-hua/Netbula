.DEFAULT_GOAL := build
# Defines which target is run when no target is specified (i.e. When running `make` alone)

.PHONY:fmt vet test race build report total 

fmt: 
	go fmt ./...

# The target 'vet' depends on target 'fmt'
vet: fmt 
	go vet ./...

test: vet
	go test -v -cover -coverprofile=c.out ./...

race : vet
	go test -v -race -cover -coverprofile=c.out ./...

build: test 
	go build -o bin/netbula main.go

report: test 
	go tool cover -html=c.out

total: test 
	go tool cover -func=c.out