# Dockerfile for nano-agent
# Simple runtime image using pre-built binary

FROM alpine:3.19

# Install dependencies for network tools
RUN apk add --no-cache \
    ca-certificates \
    bash \
    net-snmp-tools

# Create required directories
RUN mkdir -p /etc/nano-agent /var/lib/nano-agent

# Copy the pre-built binary
COPY nano-agent /usr/local/bin/nano-agent
RUN chmod +x /usr/local/bin/nano-agent

# Set working directory
WORKDIR /app

# Run nano-agent by default
ENTRYPOINT ["/usr/local/bin/nano-agent"]
CMD ["--help"]
