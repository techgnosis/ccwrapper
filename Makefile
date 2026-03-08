build:
	GOOS=darwin GOARCH=arm64 go build .

clean:
	rm -f ccwrapper-darwin-arm64

.PHONY: build clean
