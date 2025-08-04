# URL Shortener

A robust and scalable URL shortener service built with Go. This project is designed as an internal microservice, providing both gRPC and RESTful APIs for creating and managing short links.

## Features

-   **URL Shortening**: Converts long URLs into a compact, easy-to-share format.
-   **URL Redirection**: Redirects short links to their original long URLs.
-   **Dual API Support**:
    -   **gRPC**: For high-performance internal service-to-service communication.
    -   **RESTful HTTP/JSON**: For ease of use, debugging, and integration with a wider range of clients.
-   **High Performance**: Leverages Go's concurrency and a caching layer for low-latency responses.
-   **Caching**: Uses Redis with a Least Frequently Used (LFU) eviction policy to keep popular URLs hot in memory.
-   **Collision Handling**: Implements a simple and effective retry mechanism for handling short code collisions.
-   **Metrics**: Exposes metrics (e.g., collision count) for monitoring and observability.

## Project Layout

The project follows a standard Go project layout to separate concerns and improve maintainability.

-   `/cmd`: Entry point currently for the `urlshortener` CLI. The server currently runs from main.go, mainly due to the ease of embedding apidocs file.
-   `/core`: Contains the core business logic and data structures of the application, such as the `URL` struct and the `GenerateShortCode` function in `core/core.go`. This package is designed to have no external dependencies on datastores or transport layers.
-   `/datastore`: Handles all database and cache interactions. It provides an abstraction layer (`Store`) over Postgres and Redis.
-   `/rpcserver`: Defines and implements the gRPC service.
    -   `/proto`: Contains the Protobuf definition files.
-   `/httpserver`: Contains the implementation of the HTTP/REST server, including the gRPC-gateway setup.
-   `/systemtest`: Contains end-to-end system tests that run against a live instance of the service and its dependencies.
-   `/.migrations`: Database migration files.
-   `Makefile`: Contains helper commands for development tasks like running, testing, and linting.
-   `apidocs.swagger.json`: OpenAPI specification for the REST API. This file is generated automatically based on the Protobuf definitions.

## System Design and Architecture

This service is built with scalability and maintainability in mind, drawing inspiration from common system design patterns for URL shorteners.

### High-Level Diagram

```
+--------+      +----------------+      +---------------------+      +----------------+
|        |----->|                |----->|                     |----->|                |
| Client |      |  gRPC-Gateway  |      |  Go URL Shortener   |      |      Redis     |
| (User/ |      | (HTTP to gRPC) |      |       Service       |      | (Cache - LFU)  |
| Service)|<-----|                |<-----|                     |<-----|                |
+--------+      +----------------+      +---------+-----------+      +----------------+
                                                  |
                                                  |
                                                  v
                                           +------------+
                                           |            |
                                           |  Postgres  |
                                           | (Database) |
                                           |            |
                                           +------------+
```

### Capacity Estimation (Back-of-the-envelope)

To understand the system's capabilities, let's make some assumptions for a medium-sized company's internal usage.

**Assumptions:**
*   **New URLs**: 100 million new URLs per month.
*   **Read/Write Ratio**: 10:1 (10 reads for every 1 write).
*   **Short Code Length**: 6 characters using `[a-zA-Z0-9]` (62 possible characters).
*   **Average Long URL Size**: 100 bytes.

**Calculations:**
*   **Write Throughput**:
    *   100,000,000 / (30 days * 24 hours * 3600 seconds) ≈ **40 writes/sec**.
*   **Read Throughput**:
    *   40 writes/sec * 10 = **400 reads/sec**.
*   **Storage (5 years)**:
    *   `100M URLs/month * 12 months * 5 years = 6 Billion URLs`
    *   `6B URLs * (6 bytes short code + 100 bytes long URL + 8 bytes ID + indexes/overhead ≈ 200 bytes/row) ≈ 1.2 TB`
    *   This is well within the capacity of a standard Postgres instance.
*   **Cache Size (20% of daily reads)**:
    *   Assuming traffic is concentrated in 8 business hours: `400 reads/sec * 3600 sec/hr * 8 hrs/day ≈ 11.5M reads/day`
    *   `11.5M reads * 20% = 2.3M URLs in cache`
    *   `2.3M URLs * (6 bytes key + 100 bytes value + LFU metadata ≈ 150 bytes/entry) ≈ 345 MB`
    *   A Redis instance with 1-2 GB of memory would be more than sufficient to handle traffic spikes and provide excellent performance.

### Architectural Choices & Trade-offs

#### Short Code Generation

The method for generating the unique short code is a critical design choice.

*   **Our Approach: Randomized Generation with Retries**
    *   The service generates a 6-character random alphanumeric string. It then attempts to insert this into the database. If a collision occurs (the code already exists), it retries the process up to 5 times.
    *   **Why this choice?** For an internal service with the estimated traffic, this "best-effort" approach is incredibly simple to implement and is stateless. The keyspace (62^6, over 56 billion) is vast enough that collisions will be extremely rare initially. This avoids the complexity and potential single points of failure associated with other methods.
    *   **Trade-offs**: As the number of stored URLs grows into the billions, the probability of collisions increases. At a very high scale, the write latency could increase due to retries.

*   **Alternative Architectures**
    1.  **Base-62 Conversion**: Use a distributed, unique ID generator (like a database sequence or a service like Zookeeper) to produce an integer. Convert this integer to a base-62 string. This guarantees uniqueness and eliminates collisions but introduces the complexity of managing a stateful, highly-available counter.
    2.  **Pre-generation of Keys**: A separate background service generates a massive pool of unique keys and stores them in a database. The shortener service simply claims an unused key from this pool. This makes writes very fast but adds operational overhead for the key-generation service.

*   **When to Evolve?**
    *   The current approach is monitored by tracking collision metrics. A key indicator for needing an architectural change would be when the `db_query_total{query_name="AddURL", status="collision"}` counter metric starts to show a significant number of creations requiring the maximum 5 retries, or when the average number of retries per creation becomes consistently non-zero. At that point, migrating to a Base-62 conversion strategy would be the logical next step to guarantee performance at scale.

#### API Layer: gRPC-Gateway

*   **Why gRPC?** As this is an internal service, other services will be its primary consumers. gRPC offers a high-performance, low-latency communication protocol with strongly-typed contracts defined in `.proto` files. This ensures reliability and efficiency for inter-service communication.
*   **Why also REST?** The `grpc-gateway` acts as a reverse proxy, translating a RESTful JSON API into gRPC calls. This provides the best of both worlds:
    *   It allows for simple, ad-hoc querying with tools like `curl` or Postman.
    *   It simplifies integration for clients that may not have robust gRPC support (e.g., frontend web applications, simple scripts).

## Getting Started

Follow these instructions to get the project running on your local machine for development and testing.

### Prerequisites

*   Go (version 1.18+)
*   Docker & Docker Compose
*   Make

### Installation & Setup

1.  **Clone the repository:**
    ```sh
    git clone <your-repo-url>
    cd urlshortener-go
    ```

2.  **Install dependencies:**
    The project uses Go Modules, and other tools can be installed via `make` (Note: it assumes MacOS with homebrew already installed).
    ```sh
    make install.deps
    make install.tools
    ```

### Running the Application

The easiest way to run the entire stack (Go service, Postgres, Redis) is using the provided `Makefile`.

1.  **Start the services:**
    ```sh
    make run
    ```
    This command will:
    *   Spin up Postgres and Redis containers in the background using `docker-compose up -d`.
    *   Start the Go URL shortener application, which will connect to the database and cache.

2.  **Access the service:**
    The REST API is now available at `http://localhost:8080`.

    **Example: Create a short URL**
    ```sh
    curl -X POST http://localhost:8080/api/v1/shorten \
      -H "Content-Type: application/json" \
      -d '{"url": "https://github.com"}'
    ```

    **Example: Access a short URL**
    If the above command returned a short code like `aBcDeF1`, you can access it in your browser or via `curl`:
    ```sh
    curl -L http://localhost:8080/aBcDeF1
    ```
    The `-L` flag tells `curl` to follow the redirect.

### Development Commands

The `Makefile` contains several useful commands for development:

*   `make lint`: Run linters (`golangci-lint` and `buf`) to check code quality and style.
*   `make test`: Run the system tests.
*   `make generate`: Generate code from Protobuf definitions and Swagger specs (apidocs.swagger.json).
*   `make install.cli`: Build and install the command-line interface for the service.
*   `make run.cli`: Run the command-line interface.
