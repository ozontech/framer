ghz --insecure --async \
  --proto ../dumb-server/api/api.proto \
  --call test.api.TestApi/Test \
  -c 10 --total 100000 \
  -B ./ghz-data.bin localhost:9090
