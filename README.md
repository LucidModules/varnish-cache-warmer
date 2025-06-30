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
- `CACHE_URLS`: Comma-separated list of URLs to warm (optional)
    - Example: `"/,/customer/account,/sofas.html,/catalog,/checkout"`
    - If not provided, uses default URLs: `/`, `/customer/account`

## Usage

### Local Development
