FROM golang:1.22-alpine AS build-stage
WORKDIR /app
RUN apk add make
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM alpine
WORKDIR /
COPY --from=build-stage /tmp/bin/framer /framer
ENTRYPOINT ["/framer"]
