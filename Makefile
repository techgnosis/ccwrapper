build:
	GOOS=darwin GOARCH=arm64 go build -o ccwrapper ./cmd/ccwrapper

install: build
	mkdir -p ~/bin
	cp ccwrapper ~/bin/ccwrapper

clean:
	rm -f ccwrapper

.PHONY: build install clean
