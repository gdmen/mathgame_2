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
	$(GOBUILD) -o ./bin/maintenance_server ./cmd/maintenance_server/
	$(GOBUILD) -o ./bin/revalidate_word_problems ./cmd/revalidate_word_problems/
	$(GOBUILD) -o ./bin/backfill_symbolic_expression ./cmd/backfill_symbolic_expression/
	$(GOBUILD) -o ./bin/diagnose_generation ./cmd/diagnose_generation/

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
	$(RM) ./web/build/* ./web/build.next ./web/build.prev

# Emit web/src/conf.json with ONLY the public fields the frontend reads.
frontend-conf:
	python3 web/gen_frontend_conf.py $(CONF) web/src/conf.json

# Build into web/build.next, then swap it into place, so the live web/build
# (served by prod-web) is never emptied mid-build. react-scripts starts every
# build by wiping its output dir; building in place left web/build a directory
# listing for the whole npm-install+webpack window while the old server kept
# serving it (#243). The swap is two renames (sub-ms); serve re-reads per
# request, so no restart is needed and the live dir holds valid content right
# up to the swap. A failed build aborts (set -e) with web/build untouched.
build-web: frontend-conf
	cd web && npm install --force && BUILD_PATH=build.next npm run build; cd -
	cd web/src && npx prettier --write .; cd -
	$(RM) ./web/build.prev
	if [ -d ./web/build ]; then mv ./web/build ./web/build.prev; fi
	mv ./web/build.next ./web/build

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

# Fails loudly if the TLS paths are missing from $(CONF): with empty --ssl
# args, serve silently falls back to plain HTTP on 443 and every HTTPS
# client sees the site as down.
prod-web:
	set -e; \
	CERT=$$(python3 -c "import json; print(json.load(open('$(CONF)')).get('tls_cert_file',''))"); \
	KEY=$$(python3 -c "import json; print(json.load(open('$(CONF)')).get('tls_key_file',''))"); \
	if [ -z "$$CERT" ] || [ -z "$$KEY" ]; then echo "tls_cert_file/tls_key_file not set in $(CONF)" >&2; exit 1; fi; \
	cd web && serve -s build -l 443 --ssl-cert "$$CERT" --ssl-key "$$KEY"

prod-api:
	GIN_MODE=release $(GOBIN)/apiserver

# Static "down for maintenance" page on the web port; deploy/update.sh swaps
# this in for mathgame-web around the disruptive part of a deploy.
prod-maintenance:
	$(GOBIN)/maintenance_server -config $(CONF) -logtostderr
