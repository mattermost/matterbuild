FROM golang:1.14.2-alpine AS builder

RUN apk add --update --no-cache ca-certificates bash make gcc musl-dev git openssh wget curl bzr

WORKDIR /go/src/matterbuild

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN make build

################

FROM alpine:3.12.0

RUN apk --no-cache add ca-certificates

COPY --from=builder /go/src/matterbuild/dist/matterbuild /usr/local/bin/

WORKDIR /app

VOLUME /app/config
RUN mkdir -p /app/logs && touch /app/logs/matterbuild.log && chown -R 1000:1000 /app/logs/matterbuild.log

USER 1000
EXPOSE 8080

ENTRYPOINT ["matterbuild"]
