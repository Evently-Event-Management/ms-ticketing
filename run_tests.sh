#!/bin/bash

# Script to run tests for Ticketly Microservice

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Starting Ticketly Microservice Tests${NC}"

# Check if go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    exit 1
fi

# Run tests with coverage
echo -e "${YELLOW}Running tests with coverage...${NC}"

# Create coverage directory if it doesn't exist
mkdir -p coverage

# Run all tests with coverage
go test -coverprofile=coverage/coverage.out ./... && \
    go tool cover -html=coverage/coverage.out -o coverage/coverage.html

if [ $? -eq 0 ]; then
    echo -e "${GREEN}All tests passed successfully!${NC}"
    echo -e "Coverage report generated at coverage/coverage.html"
else
    echo -e "${RED}Some tests failed.${NC}"
    exit 1
fi