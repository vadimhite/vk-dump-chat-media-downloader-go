FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod .
RUN go mod download
COPY main.go .
COPY internal/ internal/
COPY cmd/ cmd/
RUN CGO_ENABLED=0 go build -o downloader .
RUN CGO_ENABLED=0 go build -o gallery ./cmd/generate-gallery/

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /build/downloader .
COPY --from=builder /build/gallery .
ENTRYPOINT ["./downloader"]
