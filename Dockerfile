
# Build stage: compile static binaries for the web server and migrate tool.
FROM golang:1.26-alpine AS build

WORKDIR /src

# Download modules first so this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/web ./cmd/web \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# Runtime stage: no shell or package manager, running as a non-root user.
# Templates, static files and migrations are embedded in the binaries.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/web /out/migrate /app/

WORKDIR /app
USER nonroot:nonroot

EXPOSE 4000
ENTRYPOINT ["/app/web"]
