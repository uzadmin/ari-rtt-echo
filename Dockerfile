# Use a pre-built Asterisk 20 image for ARM64
FROM --platform=linux/arm64 andrius/asterisk:latest

# Install additional dependencies
RUN apk update && apk add \
    go \
    git \
    curl \
    vim \
    net-tools \
    iputils \
    netcat-openbsd

# Create application directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o bin/ari-service ./cmd/ari-service
RUN go build -o bin/echo-server ./cmd/echo
RUN go build -o bin/load-test ./cmd/load_test
RUN go build -o bin/load-test-new ./cmd/load_test_new
RUN go build -o bin/ari-server ./cmd/ari-server

# Expose necessary ports
# Asterisk ports
EXPOSE 5060/udp 5060/tcp
EXPOSE 8088/tcp
EXPOSE 9090/tcp
EXPOSE 9091/tcp
# RTP port range
EXPOSE 21000-31000/tcp
EXPOSE 21000-31000/udp
EXPOSE 4000/udp

# Copy Asterisk configuration files
COPY asterisk /etc/asterisk

# Create necessary directories
RUN mkdir -p /var/log/asterisk \
    && mkdir -p /var/lib/asterisk \
    && mkdir -p /var/spool/asterisk \
    && mkdir -p /app/logs \
    && mkdir -p /app/reports

# Set permissions
RUN chown -R asterisk:asterisk /etc/asterisk \
    && chown -R asterisk:asterisk /var/log/asterisk \
    && chown -R asterisk:asterisk /var/lib/asterisk \
    && chown -R asterisk:asterisk /var/spool/asterisk \
    && chown -R asterisk:asterisk /app

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]