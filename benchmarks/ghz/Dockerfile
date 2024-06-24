FROM ubuntu:24.04
WORKDIR /app
ADD https://github.com/bojand/ghz/releases/download/v0.120.0/ghz-linux-x86_64.tar.gz ./
RUN tar xvf ghz-linux-x86_64.tar.gz
ENTRYPOINT ["/app/ghz"]
