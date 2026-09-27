# syntax=docker/dockerfile:1.7
# One build stage compiles all three binaries; each final stage ships one of
# them on distroless (no shell, no package manager, runs as nonroot).
#   docker build --target api .
#   docker build --target worker .
#   docker build --target ctl .

FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /out/ ./cmd/...

FROM gcr.io/distroless/static-debian12:nonroot AS api
COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]

FROM gcr.io/distroless/static-debian12:nonroot AS worker
COPY --from=build /out/worker /worker
EXPOSE 8080
ENTRYPOINT ["/worker"]

FROM gcr.io/distroless/static-debian12:nonroot AS ctl
COPY --from=build /out/pressroomctl /pressroomctl
ENTRYPOINT ["/pressroomctl"]
