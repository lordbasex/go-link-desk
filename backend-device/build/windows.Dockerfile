# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

# Cross-compiles the host for Windows 10 and 11 (x86-64 or ARM64) with
# llvm-mingw. libvpx, libopus and libyuv are built from source as static
# libraries,
# and the C runtime's threads and C++ library are linked in too, so the .exe
# needs no DLL beyond Windows' own (scripts/check-deps.sh windows checks).
# Its icon, manifest and version come from build/windows/desk.rc.
#
#   docker buildx build --build-arg ARCH=x86_64 --build-arg GOARCH=amd64 \
#     -f build/windows.Dockerfile --output type=local,dest=../dist/desk/windows-amd64 .
FROM mstorsjo/llvm-mingw:20261006 AS build
ARG ARCH=x86_64
ARG GOARCH=amd64
# The versions and checksums of build/macos/static-libs.sh.
ARG OPUS_VERSION=1.6.1
ARG OPUS_SHA256=6ffcb593207be92584df15b32466ed64bbec99109f007c82205f0194572411a1
ARG VPX_VERSION=1.17.0
ARG VPX_SHA256=1020f184046187baa2985dbde38e0691f49c44088bca7a1842b0236c6081dc0a
# libyuv has no releases: Debian's tarball of a revision is a fixed one.
ARG YUV_VERSION=0.0.1974.20261008
ARG YUV_SHA256=9a2e5c3e450ee5fec02276c38bf2190a93b09d3cf977e2913e1430c84ad66ad8
ENV HOST=${ARCH}-w64-mingw32 PREFIX=/deps
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl make pkg-config nasm yasm perl && rm -rf /var/lib/apt/lists/*
COPY --from=golang:1.26-bookworm /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH

# libopus, static. On ARM, Windows gives Opus no way to detect the CPU's
# features at run time (rtcd), so they are chosen when it is built.
RUN curl -fsSL https://downloads.xiph.org/releases/opus/opus-${OPUS_VERSION}.tar.gz -o /tmp/opus.tar.gz && \
    echo "${OPUS_SHA256}  /tmp/opus.tar.gz" | sha256sum -c --quiet - && \
    tar xzf /tmp/opus.tar.gz -C /tmp && cd /tmp/opus-${OPUS_VERSION} && \
    RTCD="" && if [ "$ARCH" = aarch64 ]; then RTCD=--disable-rtcd; fi && \
    ./configure --host=$HOST --prefix=$PREFIX --enable-static --disable-shared \
      --disable-doc --disable-extra-programs $RTCD CC=$HOST-clang && \
    make -j"$(nproc)" && make install

# libvpx (VP8 only), static. Its encoder threads use winpthreads, linked in
# with the rest (-static).
RUN curl -fsSL https://github.com/webmproject/libvpx/archive/refs/tags/v${VPX_VERSION}.tar.gz -o /tmp/vpx.tar.gz && \
    echo "${VPX_SHA256}  /tmp/vpx.tar.gz" | sha256sum -c --quiet - && \
    tar xzf /tmp/vpx.tar.gz -C /tmp && cd /tmp/libvpx-${VPX_VERSION} && \
    case "$ARCH" in x86_64) T=x86_64-win64-gcc ;; aarch64) T=arm64-win64-gcc ;; esac && \
    CROSS=$HOST- CC=$HOST-clang CXX=$HOST-clang++ AR=llvm-ar LD=$HOST-clang STRIP=llvm-strip \
      ./configure --target=$T --prefix=$PREFIX --enable-static --disable-shared \
        --disable-examples --disable-tools --disable-docs --disable-unit-tests \
        --enable-vp8 --disable-vp9 && \
    make -j"$(nproc)" && make install

# libyuv (the capture's scaling and color conversion), static, without
# its JPEG decoder. Only the library: its tools are not needed.
RUN curl -fsSL https://deb.debian.org/debian/pool/main/liby/libyuv/libyuv_${YUV_VERSION}.orig.tar.xz -o /tmp/yuv.tar.xz && \
    echo "${YUV_SHA256}  /tmp/yuv.tar.xz" | sha256sum -c --quiet - && \
    tar xJf /tmp/yuv.tar.xz -C /tmp && cd /tmp/libyuv-${YUV_VERSION} && \
    cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE=Release -DLIBYUV_DISABLE_JPEG=ON \
      -DCMAKE_SYSTEM_NAME=Windows -DCMAKE_SYSTEM_PROCESSOR=$ARCH \
      -DCMAKE_C_COMPILER=$HOST-clang -DCMAKE_CXX_COMPILER=$HOST-clang++ && \
    cmake --build build --target yuv && \
    cp build/libyuv.a $PREFIX/lib/ && cp -r include/libyuv include/libyuv.h $PREFIX/include/

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TAGS=""
ARG VERSION=dev
# The x.y.z Explorer shows in the file's Properties.
ARG SHORT_VERSION=0.1.0
# The resources (icon, manifest, version) as a .syso the Go linker takes
# from the package's folder. The icon's tool runs here, so it is built for
# this machine, not for GOARCH (an ARM64 build on an x86-64 runner).
RUN GOOS= GOARCH= CGO_ENABLED=0 go run ./build/macos/appicon -windows build/windows/desk.ico && \
    cd build/windows && \
    printf '#define VERSION_COMMA %s,0\n#define VERSION_TEXT "%s"\n' "$(echo "$SHORT_VERSION" | tr . ,)" "$VERSION" > version.h && \
    $HOST-windres -O coff -o /src/cmd/desk/rsrc_windows_$GOARCH.syso desk.rc
# The window's build is a Windows program (no console window opens with
# it); the headless one is a console program, for servers and scripts.
RUN SUBSYSTEM="-H windowsgui" && case " $TAGS " in *" headless "*) SUBSYSTEM="" ;; esac && \
    CGO_ENABLED=1 GOOS=windows GOARCH=$GOARCH CC=$HOST-clang CXX=$HOST-clang++ \
    PKG_CONFIG_PATH=$PREFIX/lib/pkgconfig PKG_CONFIG_LIBDIR=$PREFIX/lib/pkgconfig \
    CGO_CFLAGS="-O2 -I$PREFIX/include" CGO_LDFLAGS="-static -lpthread -L$PREFIX/lib" \
    go build -trimpath -tags "$TAGS" -ldflags "-s -w $SUBSYSTEM -X main.version=$VERSION" \
      -o /out/go-link-desk.exe ./cmd/desk

# The program the installer takes: this build's, or a signed copy of it
# from a folder given as the "exe" build context
# (make windows-package-amd64 WINDOWS_EXE_DIR=…, the Windows workflow).
FROM scratch AS exe
COPY --from=build /out/go-link-desk.exe /

# The installer (build/windows/installer.nsi, NSIS: zlib license) with the
# program, its license and the third parties' notices, which come from the
# repository's root: the "root" build context (make windows-package-amd64).
FROM debian:bookworm-slim AS installer
RUN apt-get update && apt-get install -y --no-install-recommends nsis && rm -rf /var/lib/apt/lists/*
ARG VERSION=dev
ARG SHORT_VERSION=0.1.0
ARG GOARCH=amd64
WORKDIR /nsis
COPY --from=exe /go-link-desk.exe ./
COPY --from=build /src/build/windows/desk.ico ./
COPY build/windows/installer.nsi ./
COPY --from=root LICENSE ./LICENSE.txt
COPY --from=root THIRD_PARTY_NOTICES.md ./
RUN mkdir /out && cp go-link-desk.exe /out/ && \
    makensis -V2 -DVERSION="$VERSION" -DVERSION4="$SHORT_VERSION.0" -DARCH="$GOARCH" \
      -DOUT=/out/go-link-desk-setup.exe installer.nsi

FROM scratch AS setup
COPY --from=installer /out/ /

# Only the program leaves the build (docker buildx --output type=local).
FROM scratch AS export
COPY --from=build /out/ /
