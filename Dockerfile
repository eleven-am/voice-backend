FROM golang:1.25-alpine AS builder

RUN apk add --no-cache \
    gcc \
    musl-dev \
    opus-dev \
    opusfile-dev \
    upx

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /voice-backend ./cmd/server \
    && upx --best --lzma /voice-backend

FROM alpine:3.21

RUN apk add --no-cache \
    ca-certificates \
    opus \
    opusfile \
    && adduser -D -u 1000 app

COPY --from=builder /voice-backend /usr/local/bin/voice-backend

USER app

EXPOSE 8080 50051

ENTRYPOINT ["/usr/local/bin/voice-backend"]
