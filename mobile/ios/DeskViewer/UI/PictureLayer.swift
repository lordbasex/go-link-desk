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
    private let buffers = FrameBuffers()
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
        guard let frame, let pixels = buffers.pixelBuffer(frame) else { return }
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

    /** Drops what is shown (a new display, or the session's end). */
    func clear() {
        lock.lock()
        defer { lock.unlock() }
        display.sampleBufferRenderer.flush(removingDisplayedImage: true, completionHandler: nil)
    }
}
