# Multigent self-host image.
#
# Build:
#   docker build -t ghcr.io/multigent/multigent:latest .
#
# Run:
#   docker run --rm -p 27892:27892 -v multigent-data:/data \
#     ghcr.io/multigent/multigent:latest

FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
ENV CGO_ENABLED=0
RUN go build -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" -o /out/multigent ./cmd/multigent
RUN go build -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" -o /out/mga ./cmd/mga

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    docker.io \
    git \
    openssh-client \
    tzdata \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/multigent /usr/local/bin/multigent
COPY --from=build /out/mga /usr/local/bin/mga
ENV MULTIGENT_DATA_DIR=/data
WORKDIR /data
EXPOSE 27892
ENTRYPOINT ["multigent"]
CMD ["--dir", "/data", "start", "--addr", "0.0.0.0:27892"]
