FROM --platform=$BUILDPLATFORM golang:1.21@sha256:24a09375a6216764a3eda6a25490a88ac178b5fcb9511d59d0da5ebf9e496474 AS build

WORKDIR /src
COPY . .
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    make build

FROM --platform=$BUILDPLATFORM scratch
ENV GODEBUG=netdns=go

# copy certs from build image
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# copy agent binary
COPY --from=build /src/dist/woodpecker-autoscaler /bin/

ENTRYPOINT ["/bin/woodpecker-autoscaler"]
