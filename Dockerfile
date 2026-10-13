# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -o /out/urlo ./cmd/urlo

# Separate module keeps deployment-tool dependencies out of the Web service.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd cmd/urlo-migrate && CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -o /out/urlo-migrate .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/urlo /app/urlo
COPY --from=builder /out/urlo-migrate /app/urlo-migrate

USER app

EXPOSE 8080

ENTRYPOINT ["/app/urlo"]
