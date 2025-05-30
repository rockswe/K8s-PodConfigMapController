# Dockerfile

FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -o controller ./main.go

FROM scratch
COPY --from=builder /app/controller /controller
ENTRYPOINT ["/controller"]
