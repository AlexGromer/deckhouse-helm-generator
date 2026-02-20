FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy go module files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
ARG VERSION=dev
ARG BUILD_TIME
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -o /dhg ./cmd/dhg

# Final stage
FROM alpine:3.20

RUN apk --no-cache add ca-certificates

COPY --from=builder /dhg /usr/local/bin/dhg

ENTRYPOINT ["dhg"]
