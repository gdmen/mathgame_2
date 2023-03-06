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
RM=rm -rf

all: build-api build-web

dev-api:
	$(GOBIN)/apiserver -v 3 --logtostderr 1

dev-web:
	cd web && npm start

build-api:
	cd server/api && python3 generate_models.py -c models.json && python3 generate_handlers.py -c models.json && cd -
	$(GOFMT) -s .
	$(GOBUILD) -o ./bin/apiserver ./cmd/apiserver/main.go

test: build-api test-api

test-api: server/api
	$(GOTEST) ./$^

check-swagger:
	if ! which swagger >/dev/null; then \
		go get github.com/go-swagger/go-swagger/cmd/swagger && \
		go install github.com/go-swagger/go-swagger/cmd/swagger && \
		echo "swagger installed"; \
	fi

build-docs: check-swagger
	$(SWAGGER) generate spec -o ./swagger.yaml --scan-models

dev-docs: check-swagger
	$(SWAGGER) serve -F=swagger swagger.yaml

clean:
	$(RM) ./swagger.yaml
	$(RM) ./bin/*
	$(RM) ./server/api/*.generated.go
	GOBIN=$(GOBIN) $(GOCLEAN) -testcache
	$(GOMOD) tidy
	$(RM) ./web/build/*

build-web:
	cd web && npm install && npm run build; cd -
	ln -s ../../conf.json web/src/conf.json

prod-web:
	cd web && serve -s build -l 443 --ssl-cert "/etc/letsencrypt/live/cowabunga.online/fullchain.pem" --ssl-key "/etc/letsencrypt/live/cowabunga.online/privkey.pem"

prod-api:
	GIN_MODE=release $(GOBIN)/apiserver
