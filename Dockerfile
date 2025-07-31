# Build stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /usr/src/app

# We want to populate the module cache based on the go.{mod,sum} files.
COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

# Build the Go app
RUN go build -o notion-blog cmd/cli/main.go

# Production stage
FROM alpine:latest

LABEL "com.github.actions.name"="notion-blog"
LABEL "com.github.actions.description"="Notion blog articles database to hugo-style markdown."
LABEL "repository"="https://github.com/xzebra/notion-blog"
LABEL "maintainer"="xzebra <zebrv.apps@gmail.com>"

ARG USER_UID=1000
ARG USER_GID=1000
ARG USER_NAME=runnerdocker


# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Copy the binary from the build stage to the root of the filesystem
COPY --from=builder /usr/src/app/notion-blog /notion-blog

# Make the binary executable
RUN chmod +x /notion-blog

ENTRYPOINT ["/notion-blog"]
