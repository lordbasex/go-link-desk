# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

# Builds the host for Linux (amd64 or arm64) with its C libraries: libvpx
# and libopus (static) for the encoders, libyuv (static) for the capture's
# scaling and color conversion, libva's headers only (the host loads
# libva when a session encodes H.264 on the GPU), OpenGL, X11 and Wayland for the
# Fyne window, and PipeWire's headers only (the host loads libpipewire
# when a Wayland desktop shares its screen). Debian stable's glibc keeps the binary running on older
# distributions. The target platform runs through buildx (QEMU for the
# other architecture).
#
#   docker buildx build --platform linux/arm64 --build-arg TAGS=headless \
#     -f build/linux.Dockerfile --output type=local,dest=../dist/desk/linux-arm64 .
FROM golang:1.26-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends \
      pkg-config libvpx-dev libopus-dev libyuv-dev libva-dev libgl1-mesa-dev xorg-dev \
      libwayland-dev libxkbcommon-dev wayland-protocols libpipewire-0.3-dev && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TAGS=""
ARG VERSION=dev
# libvpx, libopus and libyuv go inside the binary (a folder with only
# their .a files goes first in the search), so it runs on any distribution
# whatever versions it ships. libyuv is C++, but the functions the host
# calls need no C++ runtime. The window's GL, X11 and Wayland libraries stay
# shared: every Linux desktop has them.
RUN mkdir /static && ln -s /usr/lib/*-linux-gnu/libvpx.a /usr/lib/*-linux-gnu/libopus.a \
      /usr/lib/*-linux-gnu/libyuv.a /static/ && \
    CGO_ENABLED=1 CGO_LDFLAGS="-L/static" go build -trimpath -tags "$TAGS" \
      -ldflags "-s -w -X main.version=$VERSION" -o /out/go-link-desk ./cmd/desk

# Only the binary leaves the build (docker buildx --output type=local).
FROM scratch AS export
COPY --from=build /out/ /
