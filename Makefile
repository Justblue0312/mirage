.PHONY: build test test-unit test-integration test-ci lint clean install release

build:
	go build -o bin/mirage.exe ./cmd/mirage

test:
	go test -v -race ./...

test-unit:
	go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.out ./...

test-integration:
	-podman compose -f _examples/docker-compose.yml up --build --abort-on-container-exit --exit-code-from test-runner test-runner
	podman compose -f _examples/docker-compose.yml down -v

# Windows/macOS CI: go test in Docker with gcc
docker-test:
	docker build -t mirage-test -f Dockerfile.test .
	docker run --rm --network=host mirage-test go test -tags=integration -race -p 1 -count=1 ./...

test-ci: lint test-unit test-integration

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

install:
	go install ./cmd/mirage

release:
ifndef VERSION
	$(error VERSION is required. Usage: make release VERSION=v1.1.0)
endif
	git tag -a $(VERSION) -m "$(VERSION)"
	git push origin $(VERSION)
	gh release create $(VERSION) --title "$(VERSION)" --generate-notes
