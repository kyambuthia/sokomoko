# syntax=docker/dockerfile:1

# ---- build ----------------------------------------------------------------
FROM golang:1.25-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/sokomoko ./cmd/sokomoko

# ---- runtime --------------------------------------------------------------
# Templates and static assets are embedded in the binary, so the runtime
# image only needs the executable and CA certificates.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sokomoko /usr/local/bin/sokomoko

ENV ENV=production \
    PORT=8080 \
    LOG_FORMAT=json
EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/sokomoko"]
CMD ["serve"]
