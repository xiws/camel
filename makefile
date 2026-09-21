.PHONY: build clean publish

build:
	go build -o bin/camel ./cmd/camel

publish: build
	cp bin/camel $(HOME)/go/bin/camel

clean:
	rm -rf bin/
