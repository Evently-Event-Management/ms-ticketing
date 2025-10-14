# --- Build Stage ---
# Use the official Go image as a builder
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Install build tools and dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application, creating a static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /ms-ticketing ./main.go

# --- Final Stage ---
# Use a minimal, non-root image for the final container
FROM alpine:3.18

# Install CA certificates for HTTPS connections and Redis CLI for debugging
RUN apk add --no-cache ca-certificates tzdata redis

# Create a non-root user to run the application
RUN adduser -D -H -h /app appuser

# Create directories for the application
RUN mkdir -p /app/logs /app/migrations && chown -R appuser:appuser /app

# Copy the built binary from the builder stage
COPY --from=builder /ms-ticketing /app/ms-ticketing

# Copy migration files to the container
COPY migrations/ /app/migrations/

# Set the working directory
WORKDIR /app

# Set environment variables for migrations
ENV AUTO_MIGRATE=true
ENV MIGRATIONS_DIR=/app/migrations
ENV SEED_DATA=false

# Switch to non-root user for security
USER appuser

# Expose the application port
EXPOSE 8084

# Set the command to run the application
CMD ["/app/ms-ticketing"]
