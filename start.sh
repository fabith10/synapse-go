#!/usr/bin/env bash
set -e

# Determine project root directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd "$SCRIPT_DIR"

echo "===================================================="
echo "  Starting Agent Framework..."
echo "===================================================="

# Trap Ctrl+C (SIGINT) and SIGTERM to log shutdown
cleanup() {
    echo ""
    echo "===================================================="
    echo "  Agent Framework stopped."
    echo "===================================================="
}
trap cleanup INT TERM

# Run main application
go run cmd/main.go "$@"
