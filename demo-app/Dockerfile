# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/nimbus-demo \
    .


FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S demo \
    && adduser -S -G demo demo

WORKDIR /app

COPY --from=build /out/nimbus-demo /app/nimbus-demo

USER demo

ENV DEMO_INITIAL_VERSION=v1.0.0

EXPOSE 8080

CMD ["/app/nimbus-demo"]
