FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod .
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 go build -o downloader .

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /build/downloader .
ENTRYPOINT ["./downloader"]
