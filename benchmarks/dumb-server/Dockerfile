FROM golang:1.22-alpine AS build-stage
WORKDIR /app
RUN apk add make
COPY go.mod go.sum ./
RUN go mod download
COPY . .
WORKDIR /app/benchmarks/dumb-server
RUN make build

FROM alpine
WORKDIR /
COPY --from=build-stage /tmp/bin/dumb-server /dumb-server
ENTRYPOINT ["/dumb-server"]
