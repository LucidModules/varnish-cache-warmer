FROM golang:1.24.4-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o cache-warmer .

FROM alpine:latest as prod

RUN apk --no-cache add ca-certificates

RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app

COPY --from=builder /app/cache-warmer .

RUN chown appuser:appuser /app/cache-warmer

USER appuser

ENTRYPOINT ["./cache-warmer"]