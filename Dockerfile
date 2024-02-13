FROM golang:1.20.12 AS builder

WORKDIR /go/src/matterbuild

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 make build

################

FROM ubuntu:noble-20240127.1@sha256:bce129bec07bab56ada102d312ebcfe70463885bdf68fb32182974bd994816e0

RUN export DEBIAN_FRONTEND="noninteractive" && \
    apt-get update && \
    apt-get upgrade -y && \
    apt-get install --no-install-recommends -y ca-certificates && \
    apt-get clean all && \
    rm -rf /var/cache/apt/

COPY --from=builder /go/src/matterbuild/dist/matterbuild /usr/local/bin/

WORKDIR /app

RUN mkdir -p /app/logs && chown -R 1000:1000 /app/logs/

USER 1000
EXPOSE 8080

ENTRYPOINT ["matterbuild"]
