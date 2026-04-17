.PHONY: all lint test build deploy

all: lint test build

lint:
	go fmt ./...
	go vet ./...
	go tool staticcheck ./...
	go tool gosec ./...
	tmp=$$(mktemp) && go build -o $$tmp . && rm -f $$tmp

test: lint
	go run github.com/onsi/ginkgo/v2/ginkgo -r --race --fail-on-pending --keep-going --fail-on-empty --require-suite -p .

build: test
	docker build --platform linux/amd64 -t gfranks/rookies-bot .
	docker push gfranks/rookies-bot
	@echo "Run 'make deploy' to push changes to production."

deploy: build
	ssh geoff@192.168.2.247 "cd ~/rookies-bot && docker compose pull && docker compose up -d"
