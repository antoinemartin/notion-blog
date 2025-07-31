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
RUN go build -o notion-blog cmd/main/main.go

# Production stage
FROM alpine:latest

LABEL "com.github.actions.name"="notion-blog"
LABEL "com.github.actions.description"="Notion blog articles database to hugo-style markdown."
LABEL "repository"="https://github.com/xzebra/notion-blog"
LABEL "maintainer"="xzebra <zebrv.apps@gmail.com>"

ARG USER_UID=1001
ARG USER_GID=121
ARG USER_NAME=runnerdocker


# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create a non-root user
RUN addgroup -g ${USER_GID} -S ${USER_NAME} && \
    adduser -u ${USER_UID} -S ${USER_NAME} -G ${USER_NAME} 

# Copy the binary from the build stage to the root of the filesystem
COPY --from=builder /usr/src/app/notion-blog /notion-blog

# Make the binary executable and owned by the non-root user
RUN chmod +x /notion-blog && chown ${USER_NAME}:${USER_NAME} /notion-blog

# Switch to non-root user
USER ${USER_NAME}

ENTRYPOINT ["/notion-blog"]
