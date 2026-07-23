# multigent stable runtime base image
#
# This image contains stable OS and language dependencies plus a Linux `mga`
# fallback binary. At runtime, Multigent synchronizes the `mga` version that
# matches the server into /opt/multigent/toolchains. Fast-moving agent CLIs such
# as Codex, Claude Code, and Gemini are installed through the same toolchain
# bootstrap path.
#
# Build:
#   docker build -t multigent/runtime-base:latest \
#                -f docker/runtime-base/Dockerfile .
#
# Build with Chinese mirrors:
#   docker build --build-arg CN_MIRROR=1 \
#                -t multigent/runtime-base:latest \
#                -f docker/runtime-base/Dockerfile .

FROM --platform=$BUILDPLATFORM golang:1.26.4 AS runtime-cli-builder

ARG TARGETOS
ARG TARGETARCH
ARG CN_MIRROR=0
ARG GO_MODULE_PROXY=""

WORKDIR /src
COPY go.mod go.sum ./
RUN if [ -n "$GO_MODULE_PROXY" ]; then \
      go env -w GOPROXY="$GO_MODULE_PROXY"; \
    elif [ "$CN_MIRROR" = "1" ]; then \
      go env -w GOPROXY=https://goproxy.cn,direct GONOSUMDB='*'; \
    fi
RUN go mod download
COPY cmd/mga ./cmd/mga
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/mga ./cmd/mga

FROM ubuntu:24.04

ARG CN_MIRROR=0
ARG GO_VERSION=1.26.4
ARG TARGETARCH

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

RUN case "$TARGETARCH" in \
      amd64|arm64) go_arch="$TARGETARCH" ;; \
      *) echo "unsupported Go architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    if [ "$CN_MIRROR" = "1" ]; then \
      go_url="https://mirrors.aliyun.com/golang/go${GO_VERSION}.linux-${go_arch}.tar.gz"; \
    else \
      go_url="https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz"; \
    fi; \
    curl -fsSL "$go_url" | tar -C /usr/local -xzf -

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
        /opt/multigent/mga/bin \
    && printf '#!/bin/sh\necho "multigent: binary not mounted" >&2\nexit 1\n' \
        > /usr/local/bin/multigent \
    && chmod +x /usr/local/bin/multigent

COPY --from=runtime-cli-builder /out/mga /opt/multigent/mga/bin/mga
RUN chmod 0755 /opt/multigent/mga/bin/mga \
    && /opt/multigent/mga/bin/mga version

ENV PATH="/opt/multigent/mga/bin:${PATH}"

WORKDIR /workspace

CMD ["/bin/bash"]
