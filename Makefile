BINARY := bin/guaita
INSTALL_DIR ?= $(HOME)/.local/bin

.PHONY: build install uninstall test lint clean run

build:
	go build -ldflags="-s -w" -o $(BINARY) .

install: build
	install -d $(INSTALL_DIR)
	install -m 755 $(BINARY) $(INSTALL_DIR)/guaita

uninstall:
	rm -f $(INSTALL_DIR)/guaita

test:
	go test ./...

lint:
	go vet ./...

run:
	go run .

clean:
	rm -rf bin/
