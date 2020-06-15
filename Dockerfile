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

COPY --from=builder /go/src/matterbuild/dist/matterbuild/matterbuild /usr/local/bin/

WORKDIR /app

VOLUME /app/ssl
VOLUME /app/config

EXPOSE 8080 8443

ENTRYPOINT ["matterbuild"]
