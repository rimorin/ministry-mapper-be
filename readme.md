# Ministry Mapper Backend
testing
Ministry Mapper Backend is a Go-based application that helps manage and organize ministry territories, maps, and addresses. It leverages the PocketBase which is a open-source backend as a service (BaaS) platform.

## Table of Contents
- [Why?](#why)
- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)

## Why?

Ministry Mapper Backend is designed to avoid Firebase vendor lock-in by providing a self-hosted alternative. It is built on PocketBase, an open-source backend-as-a-service (BaaS) platform. PocketBase offers a simple and intuitive API for managing data and user authentication. It also supports real-time updates via Server-Sent Events (SSE) and job scheduling for periodic tasks. PocketBase utilizes SQLite, a lightweight and high-performance database engine.

## Features

- [x] Admin UI for managing congregations, territories, maps, and addresses.
- [x] User authentication and authorization.
- [x] Real-time map updates using Server-Sent Events (SSE).
- [x] Job scheduling for periodic tasks like territory aggregation.
- [x] Event hooks to trigger actions on data changes.

## Installation

1. Clone the repository:
    ```sh
    git clone git@github.com:rimorin/ministry-mapper-be.git
    cd ministry-mapper-be
    ```

2. Install dependencies:
    ```sh
    ./scripts/install.sh
    ```

3. Configure environment variables:
    ```sh
    cp .env.example .env
    # Edit the .env file to set your environment variables
    ```

4. Run the application:
    ```sh
    ./scripts/start.sh
    ```

## Development

1. To update Go dependencies:
    ```sh
    ./scripts/update.sh
    ```

## Deployment

This application can be deployed on any cloud platform that supports Docker containers.

Important notes:

- The application listens on port 8090 by default.
- Always use HTTPS to secure the communication between the client and the server.
- Always map /app/pb_data to a persistent volume to store the database and configuration files. This ensures that the data is not lost when the container is restarted.
- Use environment variables to configure the application. Do not hardcode sensitive information such as API keys in the source code.

### Docker Deployment

1. Build the Docker image:
```sh
docker build -t ministry-mapper .

2. Run the Docker container:
```sh
docker run -d -p 8080:8080 -v /path/to/pb_data:/app/pb_data ministry-mapper
```

## Usage

Interface with the server using [PocketBase Web SDK](https://github.com/pocketbase/js-sdk).
