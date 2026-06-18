FROM golang:1.23-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o smart-rag .

FROM alpine:3.24
# git: needed for incremental sync (gitDiff)
# ca-certificates: needed for TLS
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY --from=builder /build/smart-rag .
VOLUME ["/repo", "/data"]
ENTRYPOINT ["/app/smart-rag"]
CMD ["--repo=/repo", "--db=/data"]
