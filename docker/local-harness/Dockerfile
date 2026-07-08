FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        age \
        ca-certificates \
        git && \
    git config --system --add safe.directory /workspace/harness/source-repo && \
    git config --system --add safe.directory /workspace/harness/source-repo/.git && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

CMD ["--help"]
