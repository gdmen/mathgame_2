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
	python3 server/code_generation/generate_models.py -c server/api/models.json -o server/api
	python3 server/code_generation/generate_handlers.py -c server/api/models.json -o server/api
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
	-$(GOCMD) run ./cmd/clean_test_dbs -config test_conf.json
	$(RM) ./swagger.yaml
	$(RM) ./bin/*
	$(RM) ./server/api/*.generated.go
	GOBIN=$(GOBIN) $(GOCLEAN) -testcache
	$(GOMOD) tidy
	$(RM) ./web/build/*

build-web:
	cd web && npm install --force && npm run build; cd -
	cd web/src && npx prettier --write .; cd -
	ln -s ../../conf.json web/src/conf.json

prod-web:
	cd web && serve -s build -l 443 --ssl-cert "/etc/letsencrypt/live/mikeymath.org/fullchain.pem" --ssl-key "/etc/letsencrypt/live/mikeymath.org/privkey.pem"

prod-api:
	GIN_MODE=release $(GOBIN)/apiserver
