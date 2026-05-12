build:
	mkdir -p bin
	go build -o ./bin/ctxprop ./cmd/ctxprop

install:
	go install ./cmd/ctxprop