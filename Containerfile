FROM --platform=$BUILDPLATFORM golang:1.17 AS build

WORKDIR /src
COPY . .
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags '-s -w -extldflags "-static"' -o dist/woodpecker-autoscaler

FROM --platform=$BUILDPLATFORM scratch
ENV GODEBUG=netdns=go

# copy certs from build image
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# copy agent binary
COPY --from=build /src/dist/woodpecker-autoscaler /bin/

ENTRYPOINT ["/bin/woodpecker-autoscaler"]
