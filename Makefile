.PHONY: build test vet lint golden

build:
	mkdir -p dist
	go build -o dist/skillscan .

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

golden:
	go test ./... -run Golden
