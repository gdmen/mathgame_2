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
GOPATH=$(HOME)/go
SWAGGER=$(GOPATH)/bin/swagger

.PHONY: web

all: api

run_api:
	$(GOBIN)/apiserver > apiserver.log 2>&1

run_web:
	cd web && npm start

api: docs
	cd server/api && python3 generate_models.py -c models.json && cd -
	$(GOFMT) -s .
	$(GOBUILD) -o ./bin/apiserver ./cmd/apiserver/main.go

web:
	cd web && npm run build; cd -

test: api test-api

test-api: server/api
	$(GOTEST) ./$^

check-swagger:
	if ! which swagger >/dev/null; then \
		go get github.com/go-swagger/go-swagger/cmd/swagger && \
		go install github.com/go-swagger/go-swagger/cmd/swagger && \
		echo "swagger installed"; \
	fi

docs: check-swagger
	$(SWAGGER) generate spec -o ./swagger.yaml --scan-models

serve-docs: check-swagger
	$(SWAGGER) serve -F=swagger swagger.yaml

clean:
	$(RM) ./swagger.yaml
	$(RM) ./bin/*
	GOBIN=$(GOBIN) $(GOCLEAN) -testcache
	$(GOMOD) tidy

install:
	cd web && npm install; cd -
	ln -s ../../conf.json web/src/conf.json
