# Cache Warmer

A Go application that warms Varnish cache by making HTTP requests to predefined URLs.

## Features

- Concurrent URL warming with configurable worker pool
- Retry mechanism with exponential backoff
- Environment variable configuration
- Docker support
- Kubernetes deployment ready
- Non-root container execution for security

## Configuration

Set the following environment variable:

- `VARNISH_BASE_URL`: Base URL of your Varnish server (required)
- `GOMAXPROCS`: Maximum number of OS threads (optional, default: 2)

## Usage

### Local Development
