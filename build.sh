#!/bin/bash

# Upspeak Build Script
# Builds modular application as a single binary

set -e

# Build ui module
build_ui() {
    echo "Building ui module..."
    cd ui/web && npm run build && cd ../..
    echo "✓ ui/web/build/"
}

# Build all modules requiring build steps
build_modules() {
    build_ui
    # Add other modules here as needed
}

# Build application binary
build_app() {
    echo "Building binary..."
    go build -o bin/upspeak
    echo "✓ bin/upspeak ($(du -h bin/upspeak | cut -f1))"
}

# Full build
build() {
    build_modules
    build_app
}

# Clean build artifacts
cleanup() {
    echo "Cleaning..."
    [ -d "ui/web/build" ] && find ui/web/build -mindepth 1 ! -name '.gitkeep' -delete
    [ -d "ui/web/.svelte-kit" ] && rm -rf ui/web/.svelte-kit
    [ -f "bin/upspeak" ] && rm -f bin/upspeak
    # Add cleanup for other modules here
    echo "✓ Clean"
}

# Function to run in development mode
dev() {
    print_info "Starting development mode..."
    print_warning "Make sure your upspeak.yaml file is configured!"

    # Check if upspeak.yaml exists
    if [ ! -f "upspeak.yaml" ]; then
        print_error "upspeak.yaml file not found!"
        echo ""
        echo "Create upspeak.yaml from sample:"
        echo "  cp upspeak.sample.yaml upspeak.yaml"
        echo "  # Edit upspeak.yaml with your configuration"
        exit 1
    fi

    print_success "Configuration loaded"
    print_info "Starting application with embedded modules..."
    print_warning "Press Ctrl+C to stop"
    echo ""
    echo "Application: http://localhost:8080"
    echo "UI Module:   http://localhost:8080/ (embedded SPA)"
    echo ""
    echo "For ui module development with hot reload:"
    echo "  Run 'cd ui/web && npm run dev' in another terminal"
    echo "  Access at http://localhost:5173"
    echo ""

    go run main.go
}

# Show help
show_help() {
    cat << EOF
Upspeak Build Script

Usage: ./build.sh [command]

Commands:
  build       Build all modules and binary
  build-ui    Build ui module only
  build-app   Build binary only (uses existing module builds)
  cleanup     Clean build artifacts
  dev         Run in development mode
  help        Show this help

Architecture:
  Modular application composed into a single binary.
  Modules with build requirements (e.g., ui) are built before the binary.

Examples:
  ./build.sh build              # Full build
  ./build.sh build && ./bin/upspeak  # Build and run
  ./build.sh dev                # Development mode
  ./build.sh cleanup && ./build.sh build  # Clean rebuild

EOF
}

case "${1:-}" in
    build) build ;;
    build-ui) build_ui ;;
    build-app) build_app ;;
    cleanup|clean) cleanup ;;
    dev) dev ;;
    help|--help|-h) show_help ;;
    "") echo "Error: No command specified" && show_help && exit 1 ;;
    *) echo "Error: Unknown command: $1" && show_help && exit 1 ;;
esac
