# Container image for the Temis DMN service (temisd) — WP-46.
#
# Multi-stage: build a static binary, then ship it on a distroless base. temisd
# embeds everything it serves (web UI, OpenAPI spec, example models) via
# go:embed, so the runtime image needs nothing but the binary and CA roots.
#
# Build (version stamped from a tag or git describe):
#   docker build --build-arg VERSION=v1.2.3 -t temisd:v1.2.3 .
# Run:
#   docker run --rm -p 8080:8080 temisd:v1.2.3

FROM golang:1.24-alpine AS build
WORKDIR /src

# Cache module downloads separately from the source layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=0.0.0-dev
# CGO off → a fully static binary that runs on a scratch/distroless base.
RUN CGO_ENABLED=0 go build \
        -trimpath \
        -ldflags="-s -w -X github.com/pblumer/temis/internal/version.Version=${VERSION}" \
        -o /out/temisd ./cmd/temisd

# Distroless static: no shell, no package manager, non-root by default.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/temisd /usr/local/bin/temisd
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/temisd"]
CMD ["-addr", ":8080"]
