SWAG ?= $(shell go env GOPATH)/bin/swag
DATABASE_URL ?= postgres://notes:pass@localhost:5432/notes?sslmode=disable
TEST_DATABASE_URL ?= postgres://notes:pass@localhost:5432/notes_test?sslmode=disable
export DATABASE_URL

.PHONY: tools db-up migrate seed index unindex run run-cache test swagger lint experiment explain pool

## tools: swag, hey
tools:
	go install github.com/swaggo/swag/cmd/swag@v1.16.4
	go install github.com/rakyll/hey@latest

## db-up: PostgreSQL 16 (+pg_stat_statements) и Redis в Docker
db-up:
	docker compose up -d

## migrate: схема без доп. индексов (состояние «до»)
migrate:
	psql "$(DATABASE_URL)" -f migrations/001_schema.sql

## seed: 1 000 000 тестовых заметок
seed:
	psql "$(DATABASE_URL)" -v rows=1000000 -f scripts/seed.sql

## index / unindex: включить / откатить индексы оптимизации
index:
	psql "$(DATABASE_URL)" -f migrations/002_indexes.sql
unindex:
	psql "$(DATABASE_URL)" -f migrations/002_indexes.down.sql

## explain: планы ключевых запросов
explain:
	psql "$(DATABASE_URL)" -f scripts/explain.sql

run:
	go run ./cmd/api
run-cache:
	REDIS_ADDR=localhost:6379 go run ./cmd/api

## pool: состояние пулов (in-use / idle / ожидания)
pool:
	curl -s localhost:8080/debug/pool | python3 -m json.tool

test:
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test ./... -count=1

swagger:
	$(SWAG) fmt -g cmd/api/main.go
	$(SWAG) init -g cmd/api/main.go -o docs --parseInternal
	cp docs/swagger.yaml api/swagger.yaml
	npx --yes swagger2openapi@7 api/swagger.yaml --yaml --outfile api/openapi.yaml

lint:
	npx --yes @redocly/cli@1 lint api/openapi.yaml

## experiment: полный замер «до/после» + подбор пула (~6 минут)
experiment:
	./scripts/experiment.sh
