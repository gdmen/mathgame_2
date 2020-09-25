SHELL = /bin/sh

# Go related variables
GOCMD=go
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOINSTALL=$(GOCMD) install
GOMOD=$(GOCMD) mod
GOTEST=$(GOCMD) test
GOFMT=gofmt -w

.PHONY: web

all: api web

run_api: api
	$(GOBIN)/apiserver > apiserver.log 2>&1

run_web:
	cd web && npm start

api: docs
	$(GOBUILD) -o ./bin/apiserver ./cmd/apiserver/main.go

web:
	cd web && npm run build; cd -

test: test-api

test-api: internal/api
	$(GOTEST) ./$^

check-swagger:
	which swagger || (GO111MODULE=off go get -u github.com/go-swagger/go-swagger/cmd/swagger)

docs: check-swagger
	swagger generate spec -o ./swagger.yaml --scan-models

serve-docs: check-swagger
	swagger serve -F=swagger swagger.yaml

clean:
	$(RM) ./swagger.yaml
	$(RM) ./bin/*
	GOBIN=$(GOBIN) $(GOCLEAN)
	$(GOMOD) tidy

install:
	ln -s ../../conf.json web/src/conf.json
