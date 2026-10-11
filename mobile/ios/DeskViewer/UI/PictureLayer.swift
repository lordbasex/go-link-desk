// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import AVFoundation
import CoreMedia
import UIKit
@preconcurrency import WebRTC

/**
 * Shows the computer's picture the way the Mac app does: each decoded
 * frame goes to an AVSampleBufferDisplayLayer marked to be shown at once,
 * with no queue of its own (the lowest latency the system offers). The
 * hardware decoder's frames are already pixel buffers; a frame decoded in
 * software (VP8) is copied into one first.
 *
 * libwebrtc calls renderFrame on its decoding thread.
 */
final class PictureLayerView: UIView, RTCVideoRenderer {
    override class var layerClass: AnyClass { AVSampleBufferDisplayLayer.self }

    private var display: AVSampleBufferDisplayLayer { layer as! AVSampleBufferDisplayLayer }
    private let lock = NSLock()
    private var format: CMVideoFormatDescription?
    private var lastSize = CGSize.zero
    private var pool: CVPixelBufferPool?
    private var poolSize = CGSize.zero
    /** The picture's size changed (pixels), on the main queue. */
    var onSize: ((CGSize) -> Void)?

    override init(frame: CGRect) {
        super.init(frame: frame)
        display.videoGravity = .resize
        isUserInteractionEnabled = false
        backgroundColor = .black
    }

    required init?(coder: NSCoder) { fatalError("not used") }

    func setSize(_ size: CGSize) {
        DispatchQueue.main.async { self.onSize?(size) }
    }

    func renderFrame(_ frame: RTCVideoFrame?) {
        guard let frame, let pixels = pixelBuffer(frame) else { return }
        lock.lock()
        defer { lock.unlock() }
        let w = CVPixelBufferGetWidth(pixels), h = CVPixelBufferGetHeight(pixels)
        let size = CGSize(width: w, height: h)
        if format == nil || !CMVideoFormatDescriptionMatchesImageBuffer(format!, imageBuffer: pixels) {
            var f: CMVideoFormatDescription?
            CMVideoFormatDescriptionCreateForImageBuffer(allocator: nil, imageBuffer: pixels, formatDescriptionOut: &f)
            format = f
        }
        guard let format else { return }
        if size != lastSize {
            lastSize = size
            setSize(size)
        }
        var timing = CMSampleTimingInfo(duration: .invalid, presentationTimeStamp: CMClockGetTime(CMClockGetHostTimeClock()), decodeTimeStamp: .invalid)
        var sample: CMSampleBuffer?
        CMSampleBufferCreateReadyWithImageBuffer(allocator: nil, imageBuffer: pixels, formatDescription: format, sampleTiming: &timing, sampleBufferOut: &sample)
        guard let sample else { return }
        if let attachments = CMSampleBufferGetSampleAttachmentsArray(sample, createIfNecessary: true), CFArrayGetCount(attachments) > 0 {
            let dict = unsafeBitCast(CFArrayGetValueAtIndex(attachments, 0), to: CFMutableDictionary.self)
            CFDictionarySetValue(dict, Unmanaged.passUnretained(kCMSampleAttachmentKey_DisplayImmediately).toOpaque(),
                                 Unmanaged.passUnretained(kCFBooleanTrue).toOpaque())
        }
        let renderer = display.sampleBufferRenderer
        if renderer.status == .failed { renderer.flush() }
        renderer.enqueue(sample)
    }

    /** The frame as a pixel buffer: the decoder's own, or a copy of a software frame (I420 into NV12). */
    private func pixelBuffer(_ frame: RTCVideoFrame) -> CVPixelBuffer? {
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

    /** Drops what is shown (a new display, or the session's end). */
    func clear() {
        lock.lock()
        defer { lock.unlock() }
        display.sampleBufferRenderer.flush(removingDisplayedImage: true, completionHandler: nil)
    }
}
