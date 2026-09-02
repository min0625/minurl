FROM golang:1.26.8 AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

ARG LDFLAGS="-s -w"
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="${LDFLAGS}" -o minurl ./cmd/minurl

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/minurl /minurl

ENV MINURL_STORAGE_DSN=sqlite3:///data/minurl.sqlite3

VOLUME ["/data"]
EXPOSE 8888

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD ["/minurl", "healthcheck"]

ENTRYPOINT ["/minurl"]
