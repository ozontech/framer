#!env sh
framer load --addr=localhost:9090 \
	--inmem-requests --requests-file=./requests.bin --clients 10 \
	unlimited --count=10000000
