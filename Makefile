.PHONY: build run test web package package-web docker

build:
	go build -o bin/chatgpt2api ./cmd/server

run:
	CHATGPT2API_ADDR=:3000 go run ./cmd/server

test:
	go test ./...

web:
	cd web && npm ci && npm run build
	rm -rf web_dist && cp -R web/out web_dist

package:
	scripts/package_release.sh

package-web:
	scripts/package_release.sh --web

docker:
	docker build -t chatgpt2api-go .
