# Movie Booking Backend

A Go backend for a movie booking system, built as a foundation for exploring
concurrency, data consistency, and production infrastructure.

## Stack

- Go
- Chi
- PostgreSQL
- Redis

## Architecture

The application is currently a monolith, with PostgreSQL as the primary
persistent datastore. The project will progressively introduce the infrastructure
and concurrency problems that appear in a real booking system.

## Current State

- PostgreSQL-backed movie management
- HTTP API
- Structured application packages
- Database connection pooling
- Basic validation and error handling

## Planned

- Authentication and authorization
- Movie screenings and seat inventory
- Concurrent seat booking
- Redis-based temporary seat holds
- Background processing
- Docker
- CI/CD
- Observability
