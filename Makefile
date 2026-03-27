APP_NAME := rw
TRAY_NAME := rw-tray
BIN_DIR := bin
ENTRY := cmd/rw/main.go
TRAY_ENTRY := cmd/rw-tray/main.go

# Mac M4 (arm64)
GOOS := darwin
GOARCH := arm64

.PHONY: build build-tray build-all install clean test fmt vet run

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BIN_DIR)/$(APP_NAME) $(ENTRY)

build-tray:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BIN_DIR)/$(TRAY_NAME) $(TRAY_ENTRY)

build-all:
	GOOS=darwin GOARCH=arm64 go build -o $(BIN_DIR)/$(APP_NAME)-darwin-arm64 $(ENTRY)
	GOOS=darwin GOARCH=amd64 go build -o $(BIN_DIR)/$(APP_NAME)-darwin-amd64 $(ENTRY)
	GOOS=linux GOARCH=amd64 go build -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 $(ENTRY)
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/$(APP_NAME)-windows-amd64.exe $(ENTRY)
	GOOS=darwin GOARCH=arm64 go build -o $(BIN_DIR)/$(TRAY_NAME)-darwin-arm64 $(TRAY_ENTRY)
	GOOS=darwin GOARCH=amd64 go build -o $(BIN_DIR)/$(TRAY_NAME)-darwin-amd64 $(TRAY_ENTRY)
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/$(TRAY_NAME)-windows-amd64.exe $(TRAY_ENTRY)

install: build build-tray
	cp $(BIN_DIR)/$(APP_NAME) /usr/local/bin/$(APP_NAME)
	cp $(BIN_DIR)/$(TRAY_NAME) /usr/local/bin/$(TRAY_NAME)

clean:
	rm -rf $(BIN_DIR)
	rm -f rw rw.exe

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

run: build
	./$(BIN_DIR)/$(APP_NAME)
