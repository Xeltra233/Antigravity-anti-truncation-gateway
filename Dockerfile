# Build stage
FROM golang:alpine AS builder

WORKDIR /app

# Enable automatic Go toolchain upgrade if sub-dependencies require it
ENV GOTOOLCHAIN=auto

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates

# Copy source code and modules
COPY . .

# Build the binary targeting cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/server ./cmd/gateway

# Final stage
FROM alpine:3.21

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/bin/server /app/server

EXPOSE 8080

ENTRYPOINT ["/app/server"]
