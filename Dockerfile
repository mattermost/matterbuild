FROM golang:1.17.6 AS builder

WORKDIR /go/src/matterbuild

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 make build

################

FROM debian:buster-slim

RUN apt-get update && \
    apt-get install --no-install-recommends -y ca-certificates && \
    apt-get clean all && \
    rm -rf /var/cache/apt/

COPY --from=builder /go/src/matterbuild/dist/matterbuild /usr/local/bin/

WORKDIR /app

RUN mkdir -p /app/logs && chown -R 1000:1000 /app/logs/

USER 1000
EXPOSE 8080

ENTRYPOINT ["matterbuild"]
