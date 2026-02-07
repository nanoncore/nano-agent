# Dockerfile for nano-agent
# Simple runtime image using pre-built binary

FROM alpine:3.19

# Install dependencies for network tools
RUN apk add --no-cache \
    ca-certificates \
    bash \
    net-snmp-tools

# Create required directories and user
RUN addgroup -S nano && adduser -S -G nano nano \
    && mkdir -p /etc/nano-agent /var/lib/nano-agent \
    && chown -R nano:nano /etc/nano-agent /var/lib/nano-agent

# Copy the pre-built binary
COPY nano-agent /usr/local/bin/nano-agent
RUN chmod +x /usr/local/bin/nano-agent

# Set working directory
WORKDIR /app

USER nano

# Run nano-agent by default
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD /usr/local/bin/nano-agent --help >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/nano-agent"]
CMD ["--help"]
