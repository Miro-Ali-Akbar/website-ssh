# Cross-compiles on the build machine, so multi-arch builds for the Pis
# don't need QEMU emulation.
FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.26 AS build
ARG TARGETOS TARGETARCH TARGETVARIANT
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags="-s -w" -o /website-ssh .

# No shell, no package manager, runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /website-ssh /website-ssh
EXPOSE 2222
ENTRYPOINT ["/website-ssh"]
