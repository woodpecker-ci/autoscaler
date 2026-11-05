# docker build --rm -f docker/Dockerfile.make -t woodpecker/make:local .
FROM docker.io/golang:1.27-alpine

RUN apk add --no-cache --update make gcc binutils-gold musl-dev && \
    apk add --no-cache --repository=http://dl-cdn.alpinelinux.org/alpine/edge/main protoc

# Build packages.
COPY Makefile /

# Cache tools
RUN GOBIN=/usr/local/go/bin make -C / install-tools && \
    rm -rf /Makefile

ENV GOPATH=/tmp/go
ENV HOME=/tmp/home
ENV PATH=$PATH:/usr/local/go/bin:/tmp/go/bin

WORKDIR /build
RUN chmod -R 777 /root

CMD [ "/bin/sh" ]
