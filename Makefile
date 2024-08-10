install:
	@ go install .

format-tools:
	go install mvdan.cc/gofumpt@v0.6.0
	gofumpt -l -w .

lint: format-tools
	golangci-lint run

.PHONY: install format-tools lint