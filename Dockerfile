# -----------------------------------------------------------------------------
# The base image for building the k9s binary
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine3.24@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS build

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=$TARGETOS
ENV GOARCH=$TARGETARCH

WORKDIR /helmsman
COPY go.mod go.sum main.go Makefile ./
COPY internal internal
COPY cmd cmd
RUN apk --no-cache add --update make libx11-dev git gcc libc-dev curl \
  && make build

# -----------------------------------------------------------------------------
# Build the final Docker image for the target platform (not the build host).
# Pinning this stage to $BUILDPLATFORM would yield an amd64 runtime image (incl.
# kubectl) even for the arm64 manifest entry, so we let it default to
# $TARGETPLATFORM and use buildx's TARGETARCH to fetch the matching kubectl.
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
ARG KUBECTL_VERSION="v1.37.0"
ARG TARGETARCH

COPY --from=build /helmsman/execs/helmsman /bin/helmsman
RUN apk --no-cache add --update ca-certificates \
  && apk --no-cache add --update -t deps curl vim \
  && curl -f -L https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl -o /usr/local/bin/kubectl \
  && chmod +x /usr/local/bin/kubectl \
  && apk del --purge deps

ENTRYPOINT [ "/bin/helmsman" ]
