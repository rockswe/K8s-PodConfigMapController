# Dockerfile

# Builder stage with eBPF support
FROM golang:1.23-alpine AS builder

# Install necessary packages for eBPF compilation
RUN apk add --no-cache \
    clang \
    llvm \
    libbpf-dev \
    linux-headers \
    build-base \
    git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the controller with eBPF support
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o controller ./main.go

# Runtime stage - use minimal distroless image with necessary libraries
FROM gcr.io/distroless/static:nonroot

# Copy the controller binary
COPY --from=builder /app/controller /controller

# Copy eBPF programs (if needed at runtime)
COPY --from=builder /app/ebpf/*.c /ebpf/

# Use nonroot user for security
USER nonroot:nonroot

ENTRYPOINT ["/controller"]
