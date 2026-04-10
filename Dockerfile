# ── Build Stage ──────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /app/tas-server ./cmd/server

# ── Final Stage ───────────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S tasgroup && \
    adduser  -S tasuser -G tasgroup

WORKDIR /app

COPY --from=builder /app/tas-server .
COPY --from=builder /app/migrations ./migrations

USER tasuser

EXPOSE 8080

ENTRYPOINT ["./tas-server"]
