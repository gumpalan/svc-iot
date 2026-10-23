.PHONY: run build test build-arm64 clean

run:
	go run ./cmd/iot

build:
	mkdir -p bin
	go build -o bin/iot ./cmd/iot

test:
	go test -v ./...

build-arm64:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/iot-linux-arm64 ./cmd/iot

clean:
	rm -rf bin/ data/
