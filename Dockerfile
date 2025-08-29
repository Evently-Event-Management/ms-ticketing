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

# Create a directory for logs
RUN mkdir -p /app/logs && chown -R appuser:appuser /app

# Copy the built binary from the builder stage
COPY --from=builder /ms-ticketing /app/ms-ticketing

# Set the working directory
WORKDIR /app

# Switch to non-root user for security
USER appuser

# Expose the application port
EXPOSE 8084

# Set the command to run the application
CMD ["/app/ms-ticketing"]
