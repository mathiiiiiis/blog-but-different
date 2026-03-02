#!/bin/sh
set -e

echo "Waiting for Postgres..."
until pg_isready -h db -p 5432 -U "${DB_USER}"; do
  sleep 2
done

echo "Running database migrations..."
alembic upgrade head

exec "$@"