APP_NAME := rw
BIN_DIR := bin
ENTRY := cmd/rwcli/main.go

# Mac M4 (arm64)
GOOS := darwin
GOARCH := arm64

.PHONY: build build-all install clean test fmt vet run

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BIN_DIR)/$(APP_NAME) $(ENTRY)

build-all:
	GOOS=darwin GOARCH=arm64 go build -o $(BIN_DIR)/$(APP_NAME)-darwin-arm64 $(ENTRY)
	GOOS=darwin GOARCH=amd64 go build -o $(BIN_DIR)/$(APP_NAME)-darwin-amd64 $(ENTRY)
	GOOS=linux GOARCH=amd64 go build -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 $(ENTRY)
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/$(APP_NAME)-windows-amd64.exe $(ENTRY)

install: build
	cp $(BIN_DIR)/$(APP_NAME) /usr/local/bin/$(APP_NAME)

clean:
	rm -rf $(BIN_DIR)
	rm -f rwcli rwcli.exe

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

run: build
	./$(BIN_DIR)/$(APP_NAME)
