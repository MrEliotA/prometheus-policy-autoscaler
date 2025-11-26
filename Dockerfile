# Build stage
FROM golang:1.22 AS builder

WORKDIR /workspace

# Only copy go.mod/go.sum first to leverage Docker layer caching.
COPY go.mod ./
RUN go mod download

# Copy the rest of the source.
COPY . .

# Build the controller binary.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager main.go

# Runtime stage: use a minimal base image (distroless or similar).
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]
