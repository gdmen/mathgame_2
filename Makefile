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

# Backend config the web build reads. Override for the secret scan, which builds
# against a canary config (see test-bundle-secrets) instead of real secrets.
CONF ?= conf.json

all: build-api build-cmds build-web

dev-api:
	$(GOBIN)/apiserver -v 3 --logtostderr 1

dev-web: frontend-conf
	cd web && npm start

build-api:
	python3 server/code_generation/generate_models.py -c server/api/models.json -o server/api
	python3 server/code_generation/generate_handlers.py -c server/api/models.json -o server/api
	$(GOFMT) -s .
	$(GOBUILD) -o ./bin/apiserver ./cmd/apiserver/main.go

build-cmds: build-api
	$(GOBUILD) -o ./bin/compress_events ./cmd/compress_events/
	$(GOBUILD) -o ./bin/check_disabled_videos ./cmd/check_disabled_videos/
	$(GOBUILD) -o ./bin/update_statistics_cache ./cmd/update_statistics_cache/
	$(GOBUILD) -o ./bin/recompute_problem_difficulty ./cmd/recompute_problem_difficulty/
	$(GOBUILD) -o ./bin/recompute_problem_type_bitmap ./cmd/recompute_problem_type_bitmap/
	$(GOBUILD) -o ./bin/trim_recently_shown_problems ./cmd/trim_recently_shown_problems/

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

check-disabled-videos:
	$(GOBUILD) -o ./bin/check_disabled_videos ./cmd/check_disabled_videos/
	./bin/check_disabled_videos -config conf.json

fix-disabled-videos:
	$(GOBUILD) -o ./bin/check_disabled_videos ./cmd/check_disabled_videos/
	./bin/check_disabled_videos -config conf.json --enable

clean:
	-$(GOCMD) run ./cmd/clean_test_dbs -config test_conf.json
	$(RM) ./swagger.yaml
	$(RM) ./bin/*
	$(RM) ./server/api/*.generated.go
	GOBIN=$(GOBIN) $(GOCLEAN) -testcache
	$(GOMOD) tidy
	$(RM) ./web/build/*

# Emit web/src/conf.json with ONLY the public fields the frontend reads.
frontend-conf:
	python3 web/gen_frontend_conf.py $(CONF) web/src/conf.json

build-web: frontend-conf
	cd web && npm install --force && npm run build; cd -
	cd web/src && npx prettier --write .; cd -

# Fail if any secret value from $(CONF) (or a known secret pattern) made it into
# the public web bundle. Run after build-web; safe to run pre-deploy.
check-bundle-secrets:
	python3 web/check_bundle_secrets.py $(CONF) web/build

# Reproduce the CI bundle secret scan locally: build the web bundle against a
# canary config (unique sentinel value per field, no real secrets) and fail if
# any secret-field value leaks into web/build. Safe to run anywhere; does not
# touch the real conf.json. This is exactly what the CI workflow runs.
test-bundle-secrets:
	python3 web/gen_canary_conf.py conf.json_ canary_conf.json
	$(MAKE) build-web CONF=canary_conf.json
	$(MAKE) check-bundle-secrets CONF=canary_conf.json

# Full local parity with CI: Go tests + the web bundle secret scan.
test-all: test test-bundle-secrets

prod-web:
	cd web && serve -s build -l 443 --ssl-cert "/etc/letsencrypt/live/mikeymath.org/fullchain.pem" --ssl-key "/etc/letsencrypt/live/mikeymath.org/privkey.pem"

prod-api:
	GIN_MODE=release $(GOBIN)/apiserver
