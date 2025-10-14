#!/bin/bash

# migrate.sh - Database migration helper script

set -e

# Default values
ACTION="up"
MIGRATIONS_DIR="./migrations"
DATABASE_URL=""

print_usage() {
    echo "Usage: $0 [options]"
    echo "Options:"
    echo "  -a, --action      Migration action: up, down, version (default: up)"
    echo "  -d, --directory   Migrations directory (default: ./migrations)"
    echo "  -u, --url         Database URL (default: read from POSTGRES_DSN env var)"
    echo "  -h, --help        Display this help message"
    echo ""
    echo "Actions:"
    echo "  up        Run all pending migrations"
    echo "  down      Rollback the most recent migration"
    echo "  version   Print the current migration version"
    echo "  create    Create a new migration (requires additional name argument)"
    echo ""
    echo "Examples:"
    echo "  $0 -a up"
    echo "  $0 -a down"
    echo "  $0 -a version"
    echo "  $0 -a create -n add_new_table"
    exit 1
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    key="$1"
    case $key in
        -a|--action)
        ACTION="$2"
        shift
        shift
        ;;
        -d|--directory)
        MIGRATIONS_DIR="$2"
        shift
        shift
        ;;
        -u|--url)
        DATABASE_URL="$2"
        shift
        shift
        ;;
        -n|--name)
        MIGRATION_NAME="$2"
        shift
        shift
        ;;
        -h|--help)
        print_usage
        ;;
        *)
        echo "Unknown option: $1"
        print_usage
        ;;
    esac
done

# Use environment variable if no URL provided
if [ -z "$DATABASE_URL" ]; then
    if [ -z "$POSTGRES_DSN" ]; then
        echo "Error: No database URL provided. Set POSTGRES_DSN environment variable or use -u flag."
        exit 1
    else
        DATABASE_URL="$POSTGRES_DSN"
    fi
fi

# Ensure migrations directory exists
if [ ! -d "$MIGRATIONS_DIR" ]; then
    echo "Creating migrations directory: $MIGRATIONS_DIR"
    mkdir -p "$MIGRATIONS_DIR"
fi

# Install golang-migrate if not already installed
if ! command -v migrate &> /dev/null; then
    echo "golang-migrate is not installed. Installing..."
    if command -v go &> /dev/null; then
        go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
        export PATH=$PATH:$(go env GOPATH)/bin
    else
        echo "Error: Go is not installed. Cannot install golang-migrate."
        exit 1
    fi
fi

# Handle different actions
case $ACTION in
    up)
        echo "Running all pending migrations..."
        migrate -path "$MIGRATIONS_DIR" -database "$DATABASE_URL" up
        ;;
    down)
        echo "Rolling back the most recent migration..."
        migrate -path "$MIGRATIONS_DIR" -database "$DATABASE_URL" down 1
        ;;
    version)
        echo "Current migration version:"
        migrate -path "$MIGRATIONS_DIR" -database "$DATABASE_URL" version
        ;;
    create)
        if [ -z "$MIGRATION_NAME" ]; then
            echo "Error: Migration name is required for create action. Use -n flag."
            exit 1
        fi
        echo "Creating new migration: $MIGRATION_NAME"
        migrate create -ext sql -dir "$MIGRATIONS_DIR" -seq "$MIGRATION_NAME"
        ;;
    *)
        echo "Error: Invalid action '$ACTION'"
        print_usage
        ;;
esac

echo "Done!"