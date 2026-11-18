
# Stage 1: Build the application
FROM golang:1.27 AS builder

WORKDIR /app

# Copy dependency manifests first for better layer caching
COPY go.mod go.sum ./

RUN go mod download

# Copy application source
COPY . .

# Compile a Linux executable
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /movie-booking ./cmd/api


# Stage 2: Runtime image
FROM debian:bookworm-slim

WORKDIR /app

# Create an unprivileged user
RUN useradd --system --no-create-home appuser

# Copy only the compiled executable
COPY --from=builder --chown=appuser:appuser /movie-booking .

# Run as non-root
USER appuser

EXPOSE 3000

CMD ["./movie-booking"]

