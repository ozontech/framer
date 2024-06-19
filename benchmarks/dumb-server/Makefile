MAIN_PACKAGE_PATH := ./
BINARY_NAME := dumb-server

## build: build the application
.PHONY: build
build:
	go build -o=/tmp/bin/${BINARY_NAME} ${MAIN_PACKAGE_PATH}


## run: build and run the application
.PHONY: run
run: | build
	/tmp/bin/${BINARY_NAME}

.PHONY: pb
pb:
	protoc --go_out=./pb/ --go_opt=paths=source_relative \
		--go-grpc_out=./pb/ --go-grpc_opt=paths=source_relative \
		-I=./api api.proto \
		--go_opt=Mapi.proto=github.com/rapthead/dumb-grpc-server/pb \
		--go-grpc_opt=Mapi.proto=github.com/rapthead/dumb-grpc-server/pb
