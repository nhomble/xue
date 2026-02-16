.PHONY: build test clean fmt vet

build:
	cd src && go build -o ../bin/xue .

test:
	cd src && go test -v ./...

clean:
	rm -rf bin/

fmt:
	cd src && gofmt -w .

vet:
	cd src && go vet ./...
