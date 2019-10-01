FROM golang:1.13-alpine as builder

WORKDIR /src
RUN apk add --no-cache git musl-dev gcc
COPY . /src
RUN go build -o k8s-secret-injector ./cmd/k8s-secret-injector

#
# Finally we build the running image
#
FROM alpine

RUN apk add --no-cache dumb-init

WORKDIR /app
COPY --from=builder /src/k8s-secret-injector /app

ENV PORT=80
EXPOSE 80

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/app/k8s-secret-injector"]

