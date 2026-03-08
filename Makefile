build:
	GOOS=darwin GOARCH=arm64 go build -o ccwrapper ./cmd/ccwrapper

clean:
	rm -f ccwrapper

.PHONY: build clean
