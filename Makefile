# Define the variables needed for the initialization step 
BIN_DIR := $(CURDIR)/bin
MOCKERY_BIN_PATH := $(BIN_DIR)/mockery
ORIGINAL_PROFILE := c.out
CLEANED_PROFILE := c_cleaned.out

.DEFAULT_GOAL := build
# Defines which target is run when no target is specified (i.e. When running `make` alone)

.PHONY: mock fmt vet test race build report total clean 

# Set up the `mockery` binary  
# Make it local to the project to prevent conflicts between different versions of mockery 
$(MOCKERY_BIN_PATH):
	@mkdir -p $(BIN_DIR)
# Using the environment variable `GOBIN` to specify where to download the binary 
	GOBIN=$(BIN_DIR) go install github.com/vektra/mockery/v3@v3.7.0

mock: $(MOCKERY_BIN_PATH)
	$(MOCKERY_BIN_PATH)

fmt: mock 
	go fmt ./...

# The target 'vet' depends on target 'fmt'
vet: fmt 
	go vet ./...

test: vet
	go test -v -cover -coverprofile=$(ORIGINAL_PROFILE) ./...
# First remove the lines containing internal/mocks package
	grep -v "internal/mocks" $(ORIGINAL_PROFILE) > $(CLEANED_PROFILE)

race : vet
	go test -v -race -cover -coverprofile=$(ORIGINAL_PROFILE) ./...
	grep -v "internal/mocks" $(ORIGINAL_PROFILE) > $(CLEANED_PROFILE)

build: test 
	go build -o bin/netbula main.go

report: test 
	go tool cover -html=$(CLEANED_PROFILE)

total: test
	go tool cover -func=$(CLEANED_PROFILE)

clean:  
	rm -rf $(BIN_DIR)
	rm -rf internal/mocks