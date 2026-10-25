.PHONY: build clean publish litmus

build:
	go build -o bin/camel ./cmd/camel

publish: build
	cp bin/camel $(HOME)/go/bin/camel

litmus:
	./scripts/litmus.sh

clean:
	rm -rf bin/
