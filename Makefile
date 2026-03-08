build:
	GOOS=darwin GOARCH=arm64 go build -o agentbox-darwin-arm64 .

clean:
	rm -f agentbox-darwin-arm64

.PHONY: build clean
