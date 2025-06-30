.PHONY: build run test docker-build docker-run clean

build:
	go build -o cache-warmer .

run:
	VARNISH_BASE_URL=http://localhost:80 go run .

test:
	go test -v ./...

docker-build:
	docker build -t cache-warmer:latest .

docker-run:
	docker run --rm -e VARNISH_BASE_URL=http://host.docker.internal:80 cache-warmer:latest

clean:
	rm -f cache-warmer
	docker rmi cache-warmer:latest || true
