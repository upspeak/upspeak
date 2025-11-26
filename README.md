# Upspeak

Upspeak is a personal knowledge management system and information client designed to collect, organise, and synthesise data from web sources and your own inputs.

![Project Status: Early Stage](https://img.shields.io/badge/status-early%20stage-yellow)
![Not Production Ready](https://img.shields.io/badge/production%20ready-no-red)
[![Build and Test](https://github.com/upspeak/upspeak/actions/workflows/build-and-test.yml/badge.svg)](https://github.com/upspeak/upspeak/actions/workflows/build-and-test.yml)

## Introduction

Upspeak will let you build personal knowledge repositories into queryable information archives. These repositories can include local data (notes you write) as well as gather data from any system accessible over HTTP, allowing you to annotate the data and send replies back within their contexts.

Upspeak will integrate with tools you already use to consume or create information in multiple shared contexts, such as Matrix, Discourse, and the Fediverse.

## High Level Concepts

![High level concepts for Upspeak 0.1](./assets/high-level-concepts-0.1.png)

1. **Repository**: The core component where all data will be collected, organised, and managed. It will interface with both local and remote Archives.
2. **Archives**: Data sinks that can be local (on the user's device) or remote (web sources accessible over HTTP).
3. **Nodes and Edges**: Fundamental elements within the Repository. Nodes will represent data points, while Edges will define relationships between these points, forming a structured graph.
4. **Annotation**: Users will be able to annotate data within the Repository, linking and contextualising information to enrich their knowledge graph.
5. **Integration with External Tools**: Upspeak will integrate with tools like Matrix, Discourse, and the Fediverse, enabling seamless data flow and interaction in shared contexts.

## Develop

Upspeak uses a modular architecture where modules are composed into a single binary. Use `build.sh` to build and run the application.

### Build Commands

```bash
# Full build (modules + binary)
./build.sh build

# Build ui module only
./build.sh build-ui

# Build binary only (uses existing module builds)
./build.sh build-app

# Clean build artifacts
./build.sh cleanup

# Development mode (requires upspeak.yaml)
./build.sh dev

# Show help
./build.sh help
```

### Development Workflow

```bash
# First time setup
cp upspeak.sample.yaml upspeak.yaml
# Edit upspeak.yaml with your configuration

# Run application
./build.sh dev

# For ui module development with hot reload
cd ui/web && npm run dev
# Access at http://localhost:5173
```

## License

Upspeak is licensed under the Apache License, Version 2.0 (Apache-2.0). See the [LICENSE](LICENSE) file for the full license text.
