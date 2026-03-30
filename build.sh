#!/bin/bash

# Upspeak Build Script
# Builds the application as a single binary

set -e

# Build application binary
build_app() {
    echo "Building binary..."
    go build -o bin/upspeak
    echo "  bin/upspeak ($(du -h bin/upspeak | cut -f1))"
}

# Full build
build() {
    build_app
}

# Clean build artifacts
cleanup() {
    echo "Cleaning..."
    [ -f "bin/upspeak" ] && rm -f bin/upspeak
    echo "  Clean"
}

# Run tests
run_tests() {
    echo "Running tests..."
    go test ./...
}

# Run in development mode
dev() {
    echo "Starting development mode..."
    echo "Make sure your upspeak.yaml file is configured!"

    if [ ! -f "upspeak.yaml" ]; then
        echo "Error: upspeak.yaml file not found!"
        echo ""
        echo "Create upspeak.yaml from sample:"
        echo "  cp upspeak.sample.yaml upspeak.yaml"
        echo "  # Edit upspeak.yaml with your configuration"
        exit 1
    fi

    echo "Configuration loaded"
    echo "Starting application..."
    echo ""
    echo "API: http://localhost:8080/api/v1/"
    echo ""

    go run main.go
}

# Show help
show_help() {
    cat << EOF
Upspeak Build Script

Usage: ./build.sh [command]

Commands:
  build       Build the application binary
  build-app   Build binary only (alias for build)
  test        Run all tests
  cleanup     Clean build artifacts
  dev         Run in development mode
  help        Show this help

Examples:
  ./build.sh build              # Build
  ./build.sh build && ./bin/upspeak  # Build and run
  ./build.sh dev                # Development mode
  ./build.sh cleanup && ./build.sh build  # Clean rebuild

EOF
}

case "${1:-}" in
    build) build ;;
    build-app) build_app ;;
    test) run_tests ;;
    cleanup|clean) cleanup ;;
    dev) dev ;;
    help|--help|-h) show_help ;;
    "") echo "Error: No command specified" && show_help && exit 1 ;;
    *) echo "Error: Unknown command: $1" && show_help && exit 1 ;;
esac
