GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOBIN=bin


export GO111MODULE=on

build_web:
	GOOS=linux GOARCH=amd64 go build -o bin/webhook cmd/webhook/*.go

test_web:
	go test cmd/webhook/*.go -v

vendor:
	go mod vendor

clean:
	rm -rf vendor
	rm -rf $(GOBIN)