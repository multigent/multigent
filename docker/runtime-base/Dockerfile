# multigent stable runtime base image
#
# This image contains stable OS and language dependencies only. Fast-moving
# agent CLIs such as Codex, Claude Code, and Gemini are installed by the
# Multigent runtime toolchain bootstrap into /opt/multigent/toolchains.
#
# Build:
#   docker build -t ghcr.io/multigent/multigent/runtime-base:latest \
#                -f docker/runtime-base/Dockerfile .
#
# Build with Chinese mirrors:
#   docker build --build-arg CN_MIRROR=1 \
#                -t ghcr.io/multigent/multigent/runtime-base:latest \
#                -f docker/runtime-base/Dockerfile .

FROM ubuntu:24.04

ARG CN_MIRROR=0
ARG GO_VERSION=1.26.4

ENV DEBIAN_FRONTEND=noninteractive \
    NODE_VERSION=22 \
    IS_SANDBOX=1 \
    CODEX_UNSAFE_ALLOW_NO_SANDBOX=1 \
    MULTIGENT_TOOLCHAIN_HOME=/opt/multigent/toolchains \
    NPM_CONFIG_PREFIX=/opt/multigent/toolchains/npm

RUN if [ "$CN_MIRROR" = "1" ]; then \
      if [ -f /etc/apt/sources.list.d/ubuntu.sources ]; then \
        sed -i 's|http://archive.ubuntu.com/ubuntu|https://mirrors.aliyun.com/ubuntu|g; \
                s|http://security.ubuntu.com/ubuntu|https://mirrors.aliyun.com/ubuntu|g' \
            /etc/apt/sources.list.d/ubuntu.sources; \
      else \
        sed -i 's|http://archive.ubuntu.com/ubuntu|https://mirrors.aliyun.com/ubuntu|g; \
                s|http://security.ubuntu.com/ubuntu|https://mirrors.aliyun.com/ubuntu|g' \
            /etc/apt/sources.list; \
      fi; \
    fi

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    wget \
    git \
    jq \
    make \
    unzip \
    openssh-client \
    ripgrep \
    fzf \
    python3 \
    python3-pip \
    python3-venv \
    sqlite3 \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_VERSION}.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN if [ "$CN_MIRROR" = "1" ]; then \
      npm config set registry https://registry.npmmirror.com; \
    fi

RUN if [ "$CN_MIRROR" = "1" ]; then \
      curl -fsSL "https://mirrors.aliyun.com/golang/go${GO_VERSION}.linux-amd64.tar.gz" \
        | tar -C /usr/local -xzf -; \
    else \
      curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
        | tar -C /usr/local -xzf -; \
    fi

ENV PATH="/opt/multigent/toolchains/npm/bin:/usr/local/go/bin:/root/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
    GOPATH="/root/go" \
    GOMODCACHE="/root/go/pkg/mod"

RUN if [ "$CN_MIRROR" = "1" ]; then \
      go env -w GOPROXY=https://goproxy.cn,direct GONOSUMDB='*'; \
    fi

RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) \
         signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] \
         https://cli.github.com/packages stable main" \
        | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/multigent/toolchains/npm /opt/multigent/toolchains/markers \
    && printf '#!/bin/sh\necho "multigent: binary not mounted" >&2\nexit 1\n' \
        > /usr/local/bin/multigent \
    && chmod +x /usr/local/bin/multigent

WORKDIR /workspace

CMD ["/bin/bash"]
