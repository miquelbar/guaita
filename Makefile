BINARY := bin/guaita

.PHONY: build test lint clean run

build:
	go build -ldflags="-s -w" -o $(BINARY) .

test:
	go test ./...

lint:
	go vet ./...

run:
	go run .

clean:
	rm -rf bin/
