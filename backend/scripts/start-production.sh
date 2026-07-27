#!/bin/sh
set -eu

/app/nimbus-migrate

exec /app/nimbus-api
