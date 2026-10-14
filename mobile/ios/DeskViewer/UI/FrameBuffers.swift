// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import CoreVideo
@preconcurrency import WebRTC

/**
 * A libwebrtc frame as a pixel buffer: the hardware decoder's own, or a
 * software frame (VP8) copied into one of its pool. Not thread-safe: each
 * user (the picture, the recording) has its own.
 */
final class FrameBuffers {
    private var pool: CVPixelBufferPool?
    private var poolSize = CGSize.zero

    /** The frame as a pixel buffer: the decoder's own, or a copy of a software frame (I420 into NV12). */
    func pixelBuffer(_ frame: RTCVideoFrame) -> CVPixelBuffer? {
        if let b = frame.buffer as? RTCCVPixelBuffer,
           b.cropX == 0, b.cropY == 0, b.cropWidth == CVPixelBufferGetWidth(b.pixelBuffer), b.cropHeight == CVPixelBufferGetHeight(b.pixelBuffer) {
            return b.pixelBuffer
        }
        let i420 = frame.buffer.toI420()
        let w = Int(i420.width), h = Int(i420.height)
        guard w > 0, h > 0 else { return nil }
        if pool == nil || poolSize != CGSize(width: w, height: h) {
            let attrs: [String: Any] = [
                kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange,
                kCVPixelBufferWidthKey as String: w,
                kCVPixelBufferHeightKey as String: h,
                kCVPixelBufferIOSurfacePropertiesKey as String: [:] as [String: Any],
            ]
            var p: CVPixelBufferPool?
            CVPixelBufferPoolCreate(nil, nil, attrs as CFDictionary, &p)
            pool = p
            poolSize = CGSize(width: w, height: h)
        }
        guard let pool else { return nil }
        var out: CVPixelBuffer?
        CVPixelBufferPoolCreatePixelBuffer(nil, pool, &out)
        guard let out else { return nil }
        CVPixelBufferLockBaseAddress(out, [])
        defer { CVPixelBufferUnlockBaseAddress(out, []) }
        // Luma, row by row.
        if let dst = CVPixelBufferGetBaseAddressOfPlane(out, 0) {
            let stride = CVPixelBufferGetBytesPerRowOfPlane(out, 0)
            for y in 0..<h {
                memcpy(dst + y * stride, i420.dataY + y * Int(i420.strideY), w)
            }
        }
        // Chroma, interleaved.
        if let dst = CVPixelBufferGetBaseAddressOfPlane(out, 1) {
            let stride = CVPixelBufferGetBytesPerRowOfPlane(out, 1)
            let cw = (w + 1) / 2, ch = (h + 1) / 2
            for y in 0..<ch {
                let row = (dst + y * stride).assumingMemoryBound(to: UInt8.self)
                let u = i420.dataU + y * Int(i420.strideU), v = i420.dataV + y * Int(i420.strideV)
                for x in 0..<cw {
                    row[2 * x] = u[x]
                    row[2 * x + 1] = v[x]
                }
            }
        }
        return out
    }
}
