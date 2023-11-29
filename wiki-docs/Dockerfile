FROM python:3.9-alpine

# Install system dependencies
RUN apk update && \
    apk add --no-cache rsync git nodejs npm && \
    rm -rf /var/cache/apk/*

# Copy and install Python dependencies
COPY requirements.txt requirements.txt
RUN pip install -r requirements.txt --no-cache-dir

# Switch to a non-root user (if 'nonroot' is already created)
USER nonroot

# Build doc by default
ENTRYPOINT ["mkdocs"]
CMD ["serve", "--dev-addr", "0.0.0.0:8000"]
