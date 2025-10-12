#!/bin/bash

# Script to run tests for Ticketly Microservice

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Starting Ticketly Microservice Tests${NC}"

# Check if go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    exit 1
fi

# Create coverage directory if it doesn't exist
mkdir -p coverage

echo -e "\n${YELLOW}Running tests with verbose output...${NC}"

# Run all tests with verbose output and process each line for formatted output
go test -v -coverprofile=coverage/coverage.out ./... | awk '
    /^=== RUN/ {print $0; next}
    /^--- PASS:/ {
        testname = substr($0, 11);
        sub(/ .*$/, "", testname);
        printf "    \033[32m✓ PASSED:\033[0m %s\n", testname;
        passed++;
        next;
    }
    /^--- FAIL:/ {
        testname = substr($0, 11);
        sub(/ .*$/, "", testname);
        printf "    \033[31m✗ FAILED:\033[0m %s\n", testname;
        failed++;
        next;
    }
    /^--- SKIP:/ {
        testname = substr($0, 11);
        sub(/ .*$/, "", testname);
        printf "    \033[33m⚠ SKIPPED:\033[0m %s\n", testname;
        skipped++;
        next;
    }
    /^ok |^FAIL/ {
        printf "\033[34m%s\033[0m\n", $0;
        next;
    }
    {print $0}
    END {
        total = passed + failed + skipped;
        print "";
        print "\033[33mTest Summary:\033[0m";
        printf "Total tests: %d\n", total;
        printf "\033[32mPassed: %d\033[0m\n", passed;
        if (skipped > 0) {
            printf "\033[33mSkipped: %d\033[0m\n", skipped;
        }
        if (failed > 0) {
            printf "\033[31mFailed: %d\033[0m\n", failed;
        }
    }
'

# Save exit code from go test
TEST_EXIT_CODE=${PIPESTATUS[0]}

# Generate HTML coverage report
go tool cover -html=coverage/coverage.out -o coverage/coverage.html

if [ $TEST_EXIT_CODE -ne 0 ]; then
    echo -e "\n${RED}Some tests failed.${NC}"
else
    echo -e "\n${GREEN}All tests passed successfully!${NC}"
fi

echo -e "Coverage report generated at coverage/coverage.html"
exit $TEST_EXIT_CODE