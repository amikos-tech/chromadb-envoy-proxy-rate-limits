#generate:
#	sh ./gen_api_v3.sh

build:
	tinygo build -o ./hello.wasm -scheduler=none -target=wasi ./main.go

gotest:
	go test -v ./...

lint:
	golangci-lint run

docker-build:
	docker build -t chromago-proxy .

docker-run:
	docker run --rm -p 18000:18000 -it chromago-proxy