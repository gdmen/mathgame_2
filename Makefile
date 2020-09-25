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

run_web: web
	cd web && npm start

api: cmd/apiserver/main.go
	$(GOBUILD) -o ./bin/apiserver ./$^

web:
	cd web && npm run build; cd -

test: test-api

test-api: internal/api
	$(GOTEST) ./$^

clean:
	$(RM) ./bin/*
	GOBIN=$(GOBIN) $(GOCLEAN)
	$(GOMOD) tidy

install:
	ln -s ../../conf.json web/src/conf.json

# dist:

#apiserver_pi: src/apiserver.go
#	env GOOS=linux GOARCH=arm GOARM=5 $(GOBUILD) -o ./bin/apiserver_pi ./$^

#release: apiserver_pi ui
#	mkdir -p ./bin/release
#	rm -r ./bin/release/*
#	cp ./bin/apiserver_pi ./bin/release
#	cp -r release/* ./bin/release
#	cp -r web/build ./bin/release/ui_server

#deploy:
#	ssh -t pi@10.0.0.174 "rm -rf ./delta/*"
#	scp -r ./bin/release/* pi@10.0.0.174:./delta
