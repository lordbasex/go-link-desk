// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import AVFoundation
import AudioToolbox
import os
@preconcurrency import WebRTC

/**
 * The audio device libwebrtc plays and records through, instead of its
 * built-in one. The built-in device always opens the microphone (a voice
 * processing unit with input on) as soon as there is sound to play, which
 * would ask for the microphone and show the recording indicator the moment
 * a session opens. This one plays with a plain output unit and the playback
 * session, and only switches to the voice processing unit (echo
 * cancellation, the play-and-record session) while the person has the
 * microphone on. So the microphone is asked for when it is first turned
 * on, and never used otherwise.
 *
 * libwebrtc calls these methods on its own threads; the audio callbacks
 * run on the audio render thread.
 */
final class DeskAudioDevice: NSObject, RTCAudioDevice {
    static let sampleRate = 48_000.0
    static let ioBuffer = 0.01
    static let outputChannels = 2

    private let lock = NSRecursiveLock()
    private var delegate: RTCAudioDeviceDelegate?
    private var unit: AudioUnit?
    private var unitIsVoice = false

    private(set) var isInitialized = false
    private(set) var isPlayoutInitialized = false
    private(set) var isPlaying = false
    private(set) var isRecordingInitialized = false
    private(set) var isRecording = false

    /** The input buffer the microphone is rendered into (mono, 16 bits). */
    private var inputBuffer = [Int16](repeating: 0, count: 8192)

    /**
     * The person's microphone switch. libwebrtc starts "recording" as soon
     * as the microphone line is negotiated, even with no track on it; the
     * microphone is only opened while this is on.
     */
    var micEnabled: Bool {
        get {
            lock.lock(); defer { lock.unlock() }
            return micOn
        }
        set {
            lock.lock(); defer { lock.unlock() }
            guard micOn != newValue else { return }
            micOn = newValue
            rebuild()
        }
    }

    private var micOn = false

    /** A recording of the session takes what is played (SessionRecorder). */
    private let tapLock = OSAllocatedUnfairLock<SessionRecorder?>(initialState: nil)
    var playoutTap: SessionRecorder? {
        get { tapLock.withLock { $0 } }
        set { tapLock.withLock { $0 = newValue } }
    }

    /** Called on the main queue when the play-and-record session starts or stops. */
    var onSessionChange: ((_ recording: Bool) -> Void)?

    /** A short chime mixed into the output (the Sound sheet's test). */
    private var chimeFrames = 0
    private var chimePhase = 0.0

    #if DEBUG
    /** The level of the sound played in the last second (0-1), for the debug probe. */
    private(set) var outputLevel = 0.0
    private var levelSum = 0.0
    private var levelCount = 0
    /** Debug runs can compute the level and play silence (automated tests on a desk). */
    var silentOutput = false
    #endif

    // MARK: RTCAudioDevice

    var deviceInputSampleRate: Double { Self.sampleRate }
    var inputIOBufferDuration: TimeInterval { Self.ioBuffer }
    var inputNumberOfChannels: Int { 1 }
    var inputLatency: TimeInterval { AVAudioSession.sharedInstance().inputLatency }
    var deviceOutputSampleRate: Double { Self.sampleRate }
    var outputIOBufferDuration: TimeInterval { Self.ioBuffer }
    var outputNumberOfChannels: Int { Self.outputChannels }
    var outputLatency: TimeInterval { AVAudioSession.sharedInstance().outputLatency }

    func initialize(with delegate: RTCAudioDeviceDelegate) -> Bool {
        lock.lock(); defer { lock.unlock() }
        self.delegate = delegate
        isInitialized = true
        return true
    }

    func terminateDevice() -> Bool {
        lock.lock(); defer { lock.unlock() }
        isPlaying = false
        isRecording = false
        isPlayoutInitialized = false
        isRecordingInitialized = false
        disposeUnit()
        deactivateSession()
        delegate = nil
        isInitialized = false
        return true
    }

    func initializePlayout() -> Bool {
        lock.lock(); defer { lock.unlock() }
        isPlayoutInitialized = true
        return true
    }

    func startPlayout() -> Bool {
        lock.lock(); defer { lock.unlock() }
        isPlaying = true
        return rebuild()
    }

    func stopPlayout() -> Bool {
        lock.lock(); defer { lock.unlock() }
        isPlaying = false
        return rebuild()
    }

    func initializeRecording() -> Bool {
        lock.lock(); defer { lock.unlock() }
        isRecordingInitialized = true
        return true
    }

    func startRecording() -> Bool {
        lock.lock(); defer { lock.unlock() }
        isRecording = true
        return rebuild()
    }

    func stopRecording() -> Bool {
        lock.lock(); defer { lock.unlock() }
        isRecording = false
        return rebuild()
    }

    // MARK: Chime

    /** Plays a short two-note chime on the current output. */
    func chime() {
        lock.lock(); defer { lock.unlock() }
        chimePhase = 0
        chimeFrames = Int(Self.sampleRate * 0.5)
        if unit == nil { _ = rebuild(force: true) }
    }

    // MARK: The unit

    /** Starts the unit the current state needs: none, playback only, or voice (play and record). */
    @discardableResult
    private func rebuild(force: Bool = false) -> Bool {
        let wantVoice = isRecording && micOn
        let wantUnit = isPlaying || wantVoice || force || chimeFrames > 0
        if !wantUnit {
            disposeUnit()
            deactivateSession()
            return true
        }
        if unit != nil && unitIsVoice == wantVoice { return true }
        disposeUnit()
        configureSession(voice: wantVoice)
        return createUnit(voice: wantVoice)
    }

    private func configureSession(voice: Bool) {
        let s = AVAudioSession.sharedInstance()
        do {
            if voice {
                // Voice chat: echo cancellation, headset microphones (HFP),
                // the loudspeaker by default and never the earpiece.
                try s.setCategory(.playAndRecord, mode: .voiceChat, options: [.defaultToSpeaker, .allowBluetoothHFP, .allowBluetoothA2DP])
            } else {
                // Only listening: full quality to the speaker, AirPods (A2DP) or a wired headset.
                try s.setCategory(.playback, mode: .default, options: [])
            }
            try? s.setPreferredSampleRate(Self.sampleRate)
            try? s.setPreferredIOBufferDuration(Self.ioBuffer)
            try s.setActive(true)
        } catch {
            NSLog("go-link Desk audio session: \(error.localizedDescription)")
        }
        DispatchQueue.main.async { [weak self] in self?.onSessionChange?(voice) }
    }

    private func deactivateSession() {
        try? AVAudioSession.sharedInstance().setActive(false, options: [.notifyOthersOnDeactivation])
    }

    private func createUnit(voice: Bool) -> Bool {
        var desc = AudioComponentDescription(
            componentType: kAudioUnitType_Output,
            componentSubType: voice ? kAudioUnitSubType_VoiceProcessingIO : kAudioUnitSubType_RemoteIO,
            componentManufacturer: kAudioUnitManufacturer_Apple,
            componentFlags: 0,
            componentFlagsMask: 0
        )
        guard let component = AudioComponentFindNext(nil, &desc) else { return false }
        var newUnit: AudioUnit?
        guard AudioComponentInstanceNew(component, &newUnit) == noErr, let u = newUnit else { return false }

        var on: UInt32 = 1
        var inputOn: UInt32 = voice ? 1 : 0
        let flagSize = UInt32(MemoryLayout<UInt32>.size)
        AudioUnitSetProperty(u, kAudioOutputUnitProperty_EnableIO, kAudioUnitScope_Output, 0, &on, flagSize)
        AudioUnitSetProperty(u, kAudioOutputUnitProperty_EnableIO, kAudioUnitScope_Input, 1, &inputOn, flagSize)

        var outFormat = Self.format(channels: Self.outputChannels)
        let formatSize = UInt32(MemoryLayout<AudioStreamBasicDescription>.size)
        AudioUnitSetProperty(u, kAudioUnitProperty_StreamFormat, kAudioUnitScope_Input, 0, &outFormat, formatSize)
        var render = AURenderCallbackStruct(inputProc: renderCallback, inputProcRefCon: Unmanaged.passUnretained(self).toOpaque())
        AudioUnitSetProperty(u, kAudioUnitProperty_SetRenderCallback, kAudioUnitScope_Input, 0, &render, UInt32(MemoryLayout<AURenderCallbackStruct>.size))

        if voice {
            var inFormat = Self.format(channels: 1)
            AudioUnitSetProperty(u, kAudioUnitProperty_StreamFormat, kAudioUnitScope_Output, 1, &inFormat, formatSize)
            var input = AURenderCallbackStruct(inputProc: inputCallback, inputProcRefCon: Unmanaged.passUnretained(self).toOpaque())
            AudioUnitSetProperty(u, kAudioOutputUnitProperty_SetInputCallback, kAudioUnitScope_Global, 1, &input, UInt32(MemoryLayout<AURenderCallbackStruct>.size))
        }
        guard AudioUnitInitialize(u) == noErr else {
            AudioComponentInstanceDispose(u)
            return false
        }
        guard AudioOutputUnitStart(u) == noErr else {
            AudioUnitUninitialize(u)
            AudioComponentInstanceDispose(u)
            return false
        }
        unit = u
        unitIsVoice = voice
        return true
    }

    private func disposeUnit() {
        guard let u = unit else { return }
        unit = nil
        AudioOutputUnitStop(u)
        AudioUnitUninitialize(u)
        AudioComponentInstanceDispose(u)
        delegate?.notifyAudioOutputInterrupted()
        if unitIsVoice { delegate?.notifyAudioInputInterrupted() }
    }

    private static func format(channels: Int) -> AudioStreamBasicDescription {
        AudioStreamBasicDescription(
            mSampleRate: sampleRate,
            mFormatID: kAudioFormatLinearPCM,
            mFormatFlags: kLinearPCMFormatFlagIsSignedInteger | kLinearPCMFormatFlagIsPacked,
            mBytesPerPacket: UInt32(2 * channels),
            mFramesPerPacket: 1,
            mBytesPerFrame: UInt32(2 * channels),
            mChannelsPerFrame: UInt32(channels),
            mBitsPerChannel: 16,
            mReserved: 0
        )
    }

    // MARK: Render thread

    fileprivate func render(
        _ flags: UnsafeMutablePointer<AudioUnitRenderActionFlags>,
        _ timestamp: UnsafePointer<AudioTimeStamp>,
        _ bus: UInt32,
        _ frames: UInt32,
        _ data: UnsafeMutablePointer<AudioBufferList>?
    ) -> OSStatus {
        guard let data else { return noErr }
        let buffers = UnsafeMutableAudioBufferListPointer(data)
        if isPlaying, let d = delegate {
            let status = d.getPlayoutData(flags, timestamp, Int(bus), frames, data)
            if status != noErr {
                zero(buffers)
            } else if let tap = playoutTap, let b = buffers.first, let p = b.mData?.assumingMemoryBound(to: Int16.self) {
                tap.appendPCM(p, frames: Int(frames), channels: Int(b.mNumberChannels))
            }
        } else {
            zero(buffers)
        }
        if chimeFrames > 0 { mixChime(buffers, frames: Int(frames)) }
        #if DEBUG
        measure(buffers)
        if silentOutput { zero(buffers) }
        #endif
        return noErr
    }

    fileprivate func input(
        _ flags: UnsafeMutablePointer<AudioUnitRenderActionFlags>,
        _ timestamp: UnsafePointer<AudioTimeStamp>,
        _ bus: UInt32,
        _ frames: UInt32
    ) -> OSStatus {
        guard let u = unit, unitIsVoice, isRecording, let d = delegate, Int(frames) <= inputBuffer.count else { return noErr }
        return inputBuffer.withUnsafeMutableBytes { raw -> OSStatus in
            var list = AudioBufferList(
                mNumberBuffers: 1,
                mBuffers: AudioBuffer(mNumberChannels: 1, mDataByteSize: frames * 2, mData: raw.baseAddress)
            )
            let status = AudioUnitRender(u, flags, timestamp, bus, frames, &list)
            guard status == noErr else { return status }
            return d.deliverRecordedData(flags, timestamp, Int(bus), frames, &list, nil, nil)
        }
    }

    private func zero(_ buffers: UnsafeMutableAudioBufferListPointer) {
        for b in buffers {
            if let p = b.mData { memset(p, 0, Int(b.mDataByteSize)) }
        }
    }

    private func mixChime(_ buffers: UnsafeMutableAudioBufferListPointer, frames: Int) {
        guard let b = buffers.first, let p = b.mData?.assumingMemoryBound(to: Int16.self) else { return }
        let channels = Int(b.mNumberChannels)
        let total = Int(Self.sampleRate * 0.5)
        for i in 0..<frames where chimeFrames > 0 {
            let t = total - chimeFrames
            let freq = t < total / 2 ? 880.0 : 1320.0
            let envelope = min(1, Double(chimeFrames) / 2000) * min(1, Double(t) / 200)
            chimePhase += 2 * .pi * freq / Self.sampleRate
            let v = Int16(clamping: Int(sin(chimePhase) * 8000 * envelope))
            for c in 0..<channels {
                let idx = i * channels + c
                p[idx] = Int16(clamping: Int(p[idx]) + Int(v))
            }
            chimeFrames -= 1
        }
    }

    #if DEBUG
    private func measure(_ buffers: UnsafeMutableAudioBufferListPointer) {
        guard let b = buffers.first, let p = b.mData?.assumingMemoryBound(to: Int16.self) else { return }
        let n = Int(b.mDataByteSize) / 2
        var sum = 0.0
        for i in 0..<n {
            let s = Double(p[i]) / 32768
            sum += s * s
        }
        levelSum += sum
        levelCount += n
        if levelCount >= Int(Self.sampleRate) * Self.outputChannels {
            outputLevel = (levelSum / Double(levelCount)).squareRoot()
            levelSum = 0
            levelCount = 0
        }
    }
    #endif
}

private let renderCallback: AURenderCallback = { refCon, flags, timestamp, bus, frames, data in
    Unmanaged<DeskAudioDevice>.fromOpaque(refCon).takeUnretainedValue().render(flags, timestamp, bus, frames, data)
}

private let inputCallback: AURenderCallback = { refCon, flags, timestamp, bus, frames, _ in
    Unmanaged<DeskAudioDevice>.fromOpaque(refCon).takeUnretainedValue().input(flags, timestamp, bus, frames)
}
