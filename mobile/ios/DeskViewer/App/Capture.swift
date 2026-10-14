// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import AVFoundation
import CoreImage
import Photos
import UIKit
import VideoToolbox
@preconcurrency import WebRTC

/**
 * Screenshots and recordings of the computer's screen, made here from the
 * picture this device already gets (docs/protocol.md: the computer is
 * asked first, and shows it). They go to Photos.
 */
final class FrameTap: NSObject, RTCVideoRenderer, @unchecked Sendable {
    private let lock = NSLock()
    private let buffers = FrameBuffers()
    private var last: CVPixelBuffer?
    private var recorder: SessionRecorder?

    func setSize(_ size: CGSize) {}

    /** libwebrtc's decoding thread. */
    func renderFrame(_ frame: RTCVideoFrame?) {
        guard let frame else { return }
        lock.lock()
        defer { lock.unlock() }
        guard let pb = buffers.pixelBuffer(frame) else { return }
        last = pb
        recorder?.append(pb)
    }

    /** The picture now, as an image. */
    func snapshot() -> UIImage? {
        lock.lock()
        let pb = last
        lock.unlock()
        guard let pb else { return nil }
        let ci = CIImage(cvPixelBuffer: pb)
        guard let cg = CIContext().createCGImage(ci, from: ci.extent) else { return nil }
        return UIImage(cgImage: cg)
    }

    func record(into r: SessionRecorder?) {
        lock.lock()
        recorder = r
        lock.unlock()
    }
}

/**
 * Writes the pictures (H.264) and the sound the device plays (AAC) to an
 * MPEG-4 file. Thread-safe: pictures come from libwebrtc's decoding
 * thread, the sound from the audio render thread.
 */
final class SessionRecorder: @unchecked Sendable {
    static let maxWidth = 3840
    let url: URL
    private let q = DispatchQueue(label: "org.go-link.desk.recorder")
    private var writer: AVAssetWriter?
    private var video: AVAssetWriterInput?
    private var audio: AVAssetWriterInput?
    private var adaptor: AVAssetWriterInputPixelBufferAdaptor?
    private var transfer: VTPixelTransferSession?
    private var start = CMTime.invalid
    private var lastVideo = CMTime.invalid
    private var audioAnchor = CMTime.invalid
    private var audioSamples: Int64 = 0
    private var pcmFormat: CMAudioFormatDescription?
    private var finishing = false
    private var failed = false

    init(url: URL) { self.url = url }

    private static func now() -> CMTime { CMClockGetTime(CMClockGetHostTimeClock()) }

    func append(_ pb: CVPixelBuffer) {
        q.async { self.write(pb) }
    }

    private func open(width: Int, height: Int) -> Bool {
        if writer != nil { return true }
        if failed || width <= 0 || height <= 0 { return false }
        var w = width, h = height
        if w > Self.maxWidth {
            h = h * Self.maxWidth / w
            w = Self.maxWidth
        }
        w &= ~1
        h &= ~1
        try? FileManager.default.removeItem(at: url)
        guard let wr = try? AVAssetWriter(outputURL: url, fileType: .mp4) else {
            failed = true
            return false
        }
        let v = AVAssetWriterInput(mediaType: .video, outputSettings: [
            AVVideoCodecKey: AVVideoCodecType.h264,
            AVVideoWidthKey: w,
            AVVideoHeightKey: h,
            AVVideoCompressionPropertiesKey: [
                AVVideoAverageBitRateKey: 6_000_000,
                AVVideoMaxKeyFrameIntervalDurationKey: 2,
                AVVideoProfileLevelKey: AVVideoProfileLevelH264HighAutoLevel,
                AVVideoAllowFrameReorderingKey: false,
            ],
        ])
        v.expectsMediaDataInRealTime = true
        let ad = AVAssetWriterInputPixelBufferAdaptor(assetWriterInput: v, sourcePixelBufferAttributes: [
            kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange,
            kCVPixelBufferWidthKey as String: w,
            kCVPixelBufferHeightKey as String: h,
        ])
        guard wr.canAdd(v) else {
            failed = true
            return false
        }
        wr.add(v)
        let a = AVAssetWriterInput(mediaType: .audio, outputSettings: [
            AVFormatIDKey: kAudioFormatMPEG4AAC,
            AVSampleRateKey: DeskAudioDevice.sampleRate,
            AVNumberOfChannelsKey: DeskAudioDevice.outputChannels,
            AVEncoderBitRateKey: 128_000,
        ])
        a.expectsMediaDataInRealTime = true
        if wr.canAdd(a) {
            wr.add(a)
            audio = a
        }
        guard wr.startWriting() else {
            failed = true
            return false
        }
        start = Self.now()
        wr.startSession(atSourceTime: start)
        writer = wr
        video = v
        adaptor = ad
        var t: VTPixelTransferSession?
        VTPixelTransferSessionCreate(allocator: nil, pixelTransferSessionOut: &t)
        if let t { VTSessionSetProperty(t, key: kVTPixelTransferPropertyKey_ScalingMode, value: kVTScalingMode_Letterbox) }
        transfer = t
        return true
    }

    private func write(_ pb: CVPixelBuffer) {
        guard !finishing, open(width: CVPixelBufferGetWidth(pb), height: CVPixelBufferGetHeight(pb)),
              let writer, writer.status == .writing, let video, video.isReadyForMoreMediaData,
              let pool = adaptor?.pixelBufferPool, let transfer else { return }
        let t = Self.now()
        if lastVideo.isValid, t <= lastVideo { return }
        var dst: CVPixelBuffer?
        CVPixelBufferPoolCreatePixelBuffer(nil, pool, &dst)
        guard let dst, VTPixelTransferSessionTransferImage(transfer, from: pb, to: dst) == noErr else { return }
        if adaptor?.append(dst, withPresentationTime: t) == true { lastVideo = t }
    }

    /** The sound as played: interleaved 16-bit at 48 kHz (the audio render thread; copied at once). */
    func appendPCM(_ p: UnsafePointer<Int16>, frames: Int, channels: Int) {
        guard frames > 0, channels == DeskAudioDevice.outputChannels else { return }
        let data = Data(bytes: p, count: frames * channels * 2)
        let at = Self.now()
        q.async { self.writePCM(data, frames: frames, at: at) }
    }

    private func writePCM(_ data: Data, frames: Int, at: CMTime) {
        guard !finishing, start.isValid, at >= start, let writer, writer.status == .writing, let audio else { return }
        let rate = Int32(DeskAudioDevice.sampleRate)
        var pts = audioAnchor.isValid ? audioAnchor + CMTime(value: audioSamples, timescale: rate) : at
        if !audioAnchor.isValid || abs((at - pts).seconds) > 0.25 {
            audioAnchor = at
            audioSamples = 0
            pts = at
        }
        audioSamples += Int64(frames)
        guard audio.isReadyForMoreMediaData else { return }
        if pcmFormat == nil {
            var asbd = AudioStreamBasicDescription(
                mSampleRate: DeskAudioDevice.sampleRate, mFormatID: kAudioFormatLinearPCM,
                mFormatFlags: kLinearPCMFormatFlagIsSignedInteger | kLinearPCMFormatFlagIsPacked,
                mBytesPerPacket: UInt32(2 * DeskAudioDevice.outputChannels), mFramesPerPacket: 1,
                mBytesPerFrame: UInt32(2 * DeskAudioDevice.outputChannels), mChannelsPerFrame: UInt32(DeskAudioDevice.outputChannels),
                mBitsPerChannel: 16, mReserved: 0)
            CMAudioFormatDescriptionCreate(allocator: nil, asbd: &asbd, layoutSize: 0, layout: nil, magicCookieSize: 0, magicCookie: nil,
                                           extensions: nil, formatDescriptionOut: &pcmFormat)
        }
        guard let pcmFormat else { return }
        var block: CMBlockBuffer?
        guard CMBlockBufferCreateWithMemoryBlock(allocator: nil, memoryBlock: nil, blockLength: data.count, blockAllocator: nil,
                                                 customBlockSource: nil, offsetToData: 0, dataLength: data.count,
                                                 flags: kCMBlockBufferAssureMemoryNowFlag, blockBufferOut: &block) == noErr, let block else { return }
        data.withUnsafeBytes { raw in
            _ = CMBlockBufferReplaceDataBytes(with: raw.baseAddress!, blockBuffer: block, offsetIntoDestination: 0, dataLength: data.count)
        }
        var sample: CMSampleBuffer?
        guard CMAudioSampleBufferCreateReadyWithPacketDescriptions(allocator: nil, dataBuffer: block, formatDescription: pcmFormat,
                                                                   sampleCount: frames, presentationTimeStamp: pts, packetDescriptions: nil,
                                                                   sampleBufferOut: &sample) == noErr, let sample else { return }
        audio.append(sample)
    }

    /** Closes the file: true when it holds a picture (an empty one is removed). */
    func finish(_ done: @escaping @Sendable (Bool) -> Void) {
        q.async {
            self.finishing = true
            guard let writer = self.writer, self.lastVideo.isValid else {
                self.writer?.cancelWriting()
                try? FileManager.default.removeItem(at: self.url)
                done(false)
                return
            }
            self.video?.markAsFinished()
            self.audio?.markAsFinished()
            writer.endSession(atSourceTime: self.lastVideo)
            writer.finishWriting { done(writer.status == .completed) }
        }
    }
}

/** Photos: add-only access, asked the first time something is saved. */
enum PhotoSaver {
    static func save(image: UIImage, _ done: @escaping @Sendable (Bool) -> Void) {
        guard let png = image.pngData() else { return done(false) }
        add({ PHAssetCreationRequest.forAsset().addResource(with: .photo, data: png, options: nil) }, done)
    }

    static func save(video url: URL, _ done: @escaping @Sendable (Bool) -> Void) {
        add({ PHAssetCreationRequest.forAsset().addResource(with: .video, fileURL: url, options: nil) }, { ok in
            try? FileManager.default.removeItem(at: url)
            done(ok)
        })
    }

    private static func add(_ change: @escaping @Sendable () -> Void, _ done: @escaping @Sendable (Bool) -> Void) {
        PHPhotoLibrary.requestAuthorization(for: .addOnly) { status in
            guard status == .authorized || status == .limited else { return done(false) }
            PHPhotoLibrary.shared().performChanges(change) { ok, _ in done(ok) }
        }
    }
}
