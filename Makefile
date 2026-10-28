.PHONY: all build test run clean

APP_NAME := ya-server
MAIN_PKG := ./cmd/ya-server

all: build

build:
	@echo "==> Construyendo $(APP_NAME)..."
	go build -o $(APP_NAME) $(MAIN_PKG)

test: build
	@echo "==> Ejecutando pruebas..."
	go test -v ./...

run: test
	@echo "==> Ejecutando el servidor..."
	go run $(MAIN_PKG) serve

clean:
	@echo "==> Limpiando binarios y archivos intermedios..."
	go clean -cache -testcache
	rm -f $(APP_NAME)
