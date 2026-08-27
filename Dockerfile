FROM golang:1.27.0-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /worker ./cmd/worker

FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends docker-cli ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /worker /usr/local/bin/worker
ENTRYPOINT ["worker"]
