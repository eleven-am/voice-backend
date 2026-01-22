FROM golang:1.25-alpine AS builder

RUN apk add --no-cache upx

RUN go install github.com/swaggo/swag/v2/cmd/swag@latest

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN swag init -g cmd/server/main.go -o docs --v3.1 --parseDependency --parseInternal \
    && sed -i 's/"openapi": "3.1.0"/"openapi": "3.0.0"/g' docs/swagger.json \
    && sed -i 's/openapi: 3.1.0/openapi: 3.0.0/g' docs/swagger.yaml \
    && sed -i 's/"3.1.0"/"3.0.0"/g' docs/docs.go

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /voice-backend ./cmd/server \
    && upx --best --lzma /voice-backend

FROM alpine:3.21

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 1000 app

COPY --from=builder /voice-backend /usr/local/bin/voice-backend

USER app

EXPOSE 8080 50051

ENTRYPOINT ["/usr/local/bin/voice-backend"]
