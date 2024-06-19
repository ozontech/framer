#!env sh
cpulimit -f -l 200 -- h2load http://localhost:9090/test.api.TestService/Test \
	-n 6000000 -c 10 -m 200 \
	--data=./data \
	-H 'x-my-header-key1: my-header-val1' -H 'x-my-header-key2: my-header-val2' \
	-H 'te: trailers' -H 'content-type: application/grpc'
