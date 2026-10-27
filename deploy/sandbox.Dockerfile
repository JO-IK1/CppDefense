FROM debian:bookworm-slim
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates cmake ccache g++ gcc googletest libgtest-dev libgmock-dev make ninja-build \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid 65532 sandbox \
 && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin sandbox
COPY sandbox-tools /opt/cppdefense
RUN chmod 0555 /opt/cppdefense/timed.sh \
 && mkdir -p /opt/cppdefense/googletest/googletest /opt/cppdefense/googletest/googlemock \
 && ln -s /usr/include /opt/cppdefense/googletest/googletest/include \
 && ln -s /usr/include /opt/cppdefense/googletest/googlemock/include
USER 65532:65532
WORKDIR /workspace
LABEL org.opencontainers.image.title="CppDefense sandbox" org.opencontainers.image.version="2.4.0"
