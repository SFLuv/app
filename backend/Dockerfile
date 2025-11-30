# Use the official Go image as the base image
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Install git and other dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o main .

# Start a new stage from scratch
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /app/main ./main
COPY --from=builder /app/go.mod ./go.mod
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["./main"]