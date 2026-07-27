# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build     -trimpath     -ldflags="-s -w"     -o /out/nimbus-api     ./cmd/api

RUN CGO_ENABLED=0 GOOS=linux go build     -trimpath     -ldflags="-s -w"     -o /out/nimbus-migrate     ./cmd/migrate


FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata     && addgroup -S nimbus     && adduser -S -G nimbus nimbus

WORKDIR /app

COPY --from=build /out/nimbus-api /app/nimbus-api
COPY --from=build /out/nimbus-migrate /app/nimbus-migrate
COPY migrations /app/migrations
COPY scripts/start-production.sh /app/start-production.sh

RUN chmod +x /app/start-production.sh     && chown -R nimbus:nimbus /app

USER nimbus

ENV APP_ENV=production
ENV MIGRATIONS_DIR=/app/migrations

EXPOSE 8080

CMD ["/app/start-production.sh"]
