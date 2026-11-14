# syntax=docker/dockerfile:1
# Builder: pinned by digest. The exact Go patch comes from go.mod's toolchain
# directive (go1.27.4), because base images lag behind security releases.
FROM golang:1.27-alpine@sha256:eb61c3873c97416fff0116553fff65d67a3549f72e11d9758849a689cdd8b523 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-mod=readonly GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/toolsd ./cmd/toolsd

# Runtime: distroless, no shell, no package manager, non-root by default.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/toolsd /toolsd
USER 65532:65532
EXPOSE 8080
# No HEALTHCHECK: there is no shell to run one, and Kubernetes probes are the real check.
ENTRYPOINT ["/toolsd", "-addr=:8080"]
