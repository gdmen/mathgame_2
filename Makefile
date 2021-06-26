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

all: api

run_api:
	$(GOBIN)/apiserver > apiserver.log 2>&1

run_web:
	cd web && npm start

api: docs
	gofmt -s -w .
	$(GOBUILD) -o ./bin/apiserver ./cmd/apiserver/main.go

web:
	cd web && npm run build; cd -

test: test-api

test-api: internal/api
	$(GOTEST) ./$^

check-swagger:
	if ! which swagger; then \
		$(eval DIR := $(shell mktemp -d)) \
		git clone https://github.com/go-swagger/go-swagger "$(DIR)"; \
		cd "$(DIR)"; \
		git checkout v0.26.0; \
		pwd; \
		go install -ldflags "-X github.com/go-swagger/go-swagger/cmd/swagger/commands.Version=$(git describe --tags) -X github.com/go-swagger/go-swagger/cmd/swagger/commands.Commit=$(git rev-parse HEAD)" ./cmd/swagger; \
		cd -; \
		rm -rf "$(DIR)"; \
	fi

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
	cd web && npm install; cd -
	ln -s ../../conf.json web/src/conf.json
