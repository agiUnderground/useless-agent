# syntax=docker/dockerfile:1
FROM golang:1-alpine AS builder
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /docker-controller .

FROM alpine:3
COPY --from=builder /docker-controller /usr/local/bin/docker-controller
USER 65534:65534
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/docker-controller"]
