FROM debian:bookworm-slim
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates cmake g++ gcc make ninja-build \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid 65532 sandbox \
 && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin sandbox
USER 65532:65532
WORKDIR /workspace
LABEL org.opencontainers.image.title="CppDefense sandbox" org.opencontainers.image.version="2.1.0"
