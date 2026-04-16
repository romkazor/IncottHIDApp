BINARY := IncottHIDApp.exe
LDFLAGS := -H windowsgui

.PHONY: all build test vet clean run

all: build

# Build depends on test — binary is only produced if tests pass
build: test
	go build -o $(BINARY) -ldflags="$(LDFLAGS)" .

test:
	go test -v ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY) incott.log

run: build
	./$(BINARY)
