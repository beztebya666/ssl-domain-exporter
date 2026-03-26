GO ?= go
NPM ?= npm
LDFLAGS ?=

.PHONY: test build frontend-install frontend-build frontend-audit audit ci release-check docker-build

test:
	$(GO) test ./...

build:
	$(GO) build -ldflags "$(LDFLAGS)" ./cmd/server

frontend-install:
	$(NPM) run frontend:install

frontend-build:
	$(NPM) run build

frontend-audit:
	$(NPM) --prefix frontend audit

audit:
	$(NPM) audit
	$(NPM) --prefix frontend audit

ci: test build frontend-build

release-check: test build audit frontend-build

docker-build:
	docker build -t ssl-domain-exporter .
