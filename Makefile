.PHONY: build test test-unit test-integration test-ci lint clean install release

build:
	go build -o bin/mirage.exe ./cmd/mirage

test:
	go test -v -race ./...

test-unit:
	go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.out ./...

test-integration:
	podman compose -f _examples/docker-compose.yml up -d --wait
	@MIRAGE_TEST_DATABASE_URL="postgres://test:test@localhost:15432/mirage_test?sslmode=disable" \
		go test -tags=integration -race -count=1 ./...
	podman compose -f _examples/docker-compose.yml down -v

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
