// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import SwiftUI
import UIKit
@preconcurrency import WebRTC

/**
 * The computer's screen: its picture (decoded by the hardware, shown at
 * once by PictureLayerView), its pointer, and the fingers, trackpad, mouse
 * and keyboard of this device.
 *
 * Fingers, trackpad mode (the default): one finger slides the pointer, a
 * tap clicks, a long press then a slide drags; two fingers: a tap is a
 * right click, a pinch zooms and, zoomed in, a slide moves the view;
 * three fingers scroll. Touch mode: the pointer jumps to the finger, a
 * tap clicks there, and two fingers scroll.
 *
 * An iPad's trackpad or mouse moves the pointer directly, its buttons
 * click and its scrolling scrolls. A hardware keyboard's keys go by
 * position (the same place on the computer's keyboard); the on-screen
 * keyboard sends the text typed.
 */
final class RemoteScreenView: UIView, UIKeyInput, UIGestureRecognizerDelegate {
    var session: DeskSession?
    var touchMode = false
    /** Locked: nothing is sent and the view stays still (to watch). */
    var locked = false {
        didSet { if locked && !oldValue { releaseAll() } }
    }
    /** The computer is a Mac: the bar says ⌘ and ⌥ instead of Win and Alt. */
    var mac = false {
        didSet { if mac != oldValue { accessory.relabel(mac: mac) } }
    }
    /** The bar over the on-screen keyboard: the keys it lacks. */
    private lazy var accessory = KeyBar(screen: self)
    /** Called when the person dismisses the on-screen keyboard. */
    var onKeyboardHidden: (() -> Void)?

    private let videoView = PictureLayerView(frame: .zero)
    private let cursorView = UIImageView()
    private var track: RTCVideoTrack?
    private var videoSize = CGSize.zero
    private var pointer = Pointer()
    private var zoom = Zoom()
    private var dragging = false
    private var lastPan: (CGPoint, CFTimeInterval)?
    private var lastWant = CGSize.zero
    private var wantTimer: Timer?
    private var cursorShape: CursorShape?
    private var pointsWidth = 0
    /** Modifiers latched from the bar (Ctrl, Alt, Cmd), for the next key. */
    private(set) var latched: UInt8 = 0
    var onLatch: ((UInt8) -> Void)?
    /** Hardware keys held now (wire keycode), so a lost focus lets go of them. */
    private var held: Set<UInt16> = []
    private var softKeyboard = false
    private let hiddenKeyboard = UIView(frame: .zero)

    override init(frame: CGRect) {
        super.init(frame: frame)
        backgroundColor = .black
        clipsToBounds = true
        isMultipleTouchEnabled = true
        videoView.onSize = { [weak self] size in
            self?.videoSize = size
            self?.setNeedsLayout()
        }
        addSubview(videoView)
        cursorView.isUserInteractionEnabled = false
        cursorView.isHidden = true
        addSubview(cursorView)
        gestures()
    }

    required init?(coder: NSCoder) { fatalError("not used") }

    // MARK: - Picture

    func show(_ t: RTCVideoTrack?) {
        guard t !== track else { return }
        track?.remove(videoView)
        track = t
        if t == nil { videoView.clear() }
        t?.add(videoView)
    }

    /** Where the whole picture is, fitted in the view (no zoom). */
    private var fitRect: Rect {
        let view = Rect(x: 0, y: 0, width: bounds.width, height: bounds.height)
        guard videoSize.width > 0 else { return view }
        return view.fit(videoSize.width, videoSize.height)
    }

    /** Where the picture is now, with the zoom. */
    private var pictureRect: Rect { zoom.apply(fitRect) }

    override func layoutSubviews() {
        super.layoutSubviews()
        let r = pictureRect
        videoView.frame = CGRect(x: r.x, y: r.y, width: r.width, height: r.height)
        placeCursor()
        wantSize()
    }

    /** Tells the computer the size the picture is shown at, in pixels (once the view settles). */
    private func wantSize() {
        let scale = traitCollection.displayScale
        let want = CGSize(width: (bounds.width * scale).rounded(), height: (bounds.height * scale).rounded())
        guard want.width > 0, want != lastWant else { return }
        lastWant = want
        wantTimer?.invalidate()
        wantTimer = Timer.scheduledTimer(withTimeInterval: 0.4, repeats: false) { [weak self] _ in
            MainActor.assumeIsolated { self?.session?.setVideoWant(width: Int(want.width), height: Int(want.height)) }
        }
    }

    // MARK: - Pointer

    func setCursor(_ shape: CursorShape?, pointsWidth: Int) {
        self.pointsWidth = pointsWidth
        guard shape != cursorShape else { return }
        cursorShape = shape
        cursorView.image = shape.flatMap { UIImage(data: $0.png) }
        placeCursor()
    }

    /** The computer moved its pointer by itself (or another viewer did). */
    func hostPointer(x: Double, y: Double, visible: Bool) {
        guard !dragging, lastPan == nil else { return }
        pointer = Pointer(x: x, y: y)
        cursorView.alpha = visible ? 1 : 0
        placeCursor()
    }

    private func placeCursor() {
        let r = pictureRect
        guard videoSize.width > 0 else {
            cursorView.isHidden = true
            return
        }
        let p = pointer.inView(r)
        // The image is sized in the computer's points: the same share of the picture here.
        let perPoint = pointsWidth > 0 ? r.width / Double(pointsWidth) : r.width / max(videoSize.width, 1)
        var w = 18.0, h = 18.0, hx = 0.0, hy = 0.0
        if let s = cursorShape, s.width > 0, s.height > 0 {
            w = s.width * perPoint
            h = s.height * perPoint
            let px = s.pixels > 0 ? s.pixels / s.width : 1 // image pixels per point
            hx = s.hotX / px * perPoint
            hy = s.hotY / px * perPoint
        }
        // Never tiny on a phone.
        let grow = max(1, 14 / max(w, 1))
        w *= grow; h *= grow; hx *= grow; hy *= grow
        cursorView.isHidden = cursorView.image == nil
        cursorView.frame = CGRect(x: p.x - hx, y: p.y - hy, width: w, height: h)
    }

    private func movePointer() {
        session?.pointer(x: pointer.x, y: pointer.y)
        // Zoomed in, the view follows the pointer.
        let p = pointer.inView(pictureRect)
        let before = zoom
        zoom.follow(px: p.x, py: p.y, view: Rect(x: 0, y: 0, width: bounds.width, height: bounds.height), fit: fitRect)
        if zoom != before { setNeedsLayout() }
        placeCursor()
    }

    // MARK: - Gestures

    private func gestures() {
        let pan = UIPanGestureRecognizer(target: self, action: #selector(onePan(_:)))
        pan.maximumNumberOfTouches = 1
        pan.allowedTouchTypes = [NSNumber(value: UITouch.TouchType.direct.rawValue)]
        addGestureRecognizer(pan)

        let tap = UITapGestureRecognizer(target: self, action: #selector(tap(_:)))
        tap.allowedTouchTypes = [NSNumber(value: UITouch.TouchType.direct.rawValue)]
        addGestureRecognizer(tap)

        let twoTap = UITapGestureRecognizer(target: self, action: #selector(twoFingerTap(_:)))
        twoTap.numberOfTouchesRequired = 2
        addGestureRecognizer(twoTap)

        let two = UIPanGestureRecognizer(target: self, action: #selector(twoPan(_:)))
        two.minimumNumberOfTouches = 2
        two.maximumNumberOfTouches = 2
        two.delegate = self
        addGestureRecognizer(two)

        let three = UIPanGestureRecognizer(target: self, action: #selector(threePan(_:)))
        three.minimumNumberOfTouches = 3
        three.maximumNumberOfTouches = 3
        addGestureRecognizer(three)

        let pinch = UIPinchGestureRecognizer(target: self, action: #selector(pinch(_:)))
        pinch.delegate = self
        addGestureRecognizer(pinch)

        let long = UILongPressGestureRecognizer(target: self, action: #selector(longPress(_:)))
        long.minimumPressDuration = 0.4
        long.allowedTouchTypes = [NSNumber(value: UITouch.TouchType.direct.rawValue)]
        long.delegate = self
        addGestureRecognizer(long)

        // An iPad's trackpad or mouse: the pointer follows it; scrolling scrolls.
        addGestureRecognizer(UIHoverGestureRecognizer(target: self, action: #selector(hover(_:))))
        let wheel = UIPanGestureRecognizer(target: self, action: #selector(wheel(_:)))
        wheel.allowedScrollTypesMask = .all
        wheel.allowedTouchTypes = []
        addGestureRecognizer(wheel)
    }

    override func gestureRecognizerShouldBegin(_ g: UIGestureRecognizer) -> Bool {
        !locked
    }

    func gestureRecognizer(_ g: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer) -> Bool {
        // Pinch and two-finger scroll together; a long press hands over to the slide.
        (g is UIPinchGestureRecognizer && other is UIPanGestureRecognizer) || (g is UIPanGestureRecognizer && other is UIPinchGestureRecognizer)
            || g is UILongPressGestureRecognizer || other is UILongPressGestureRecognizer
    }

    @objc private func onePan(_ g: UIPanGestureRecognizer) {
        let loc = g.location(in: self)
        switch g.state {
        case .began:
            lastPan = (loc, CACurrentMediaTime())
            if touchMode { place(loc) }
        case .changed:
            if touchMode {
                place(loc)
            } else if let (last, t) = lastPan {
                let now = CACurrentMediaTime()
                let r = pictureRect
                pointer.slide(dx: loc.x - last.x, dy: loc.y - last.y, dt: now - t, pictureWidth: r.width, pictureHeight: r.height)
                lastPan = (loc, now)
                movePointer()
            }
        default:
            lastPan = nil
        }
    }

    private func place(_ loc: CGPoint) {
        pointer.place(viewX: loc.x, viewY: loc.y, picture: pictureRect)
        movePointer()
    }

    @objc private func tap(_ g: UITapGestureRecognizer) {
        if touchMode { place(g.location(in: self)) }
        click(.left)
    }

    @objc private func twoFingerTap(_ g: UITapGestureRecognizer) {
        if touchMode {
            // Right click where the fingers are.
            place(g.location(in: self))
        }
        click(.right)
    }

    private func click(_ b: InputPacket.Button) {
        session?.button(b, down: true, x: pointer.x, y: pointer.y)
        session?.button(b, down: false, x: pointer.x, y: pointer.y)
    }

    /** A long press holds the left button; the slide that follows drags. */
    @objc private func longPress(_ g: UILongPressGestureRecognizer) {
        switch g.state {
        case .began:
            if touchMode { place(g.location(in: self)) }
            dragging = true
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            session?.button(.left, down: true, x: pointer.x, y: pointer.y)
        case .ended, .cancelled, .failed:
            if dragging {
                dragging = false
                session?.button(.left, down: false, x: pointer.x, y: pointer.y)
            }
        default:
            break
        }
    }

    /** Two fingers: the view while zoomed in (trackpad mode), or scrolling (touch mode). */
    @objc private func twoPan(_ g: UIPanGestureRecognizer) {
        let d = g.translation(in: self)
        g.setTranslation(.zero, in: self)
        if touchMode {
            scroll(d)
        } else if zoom.scale > 1 {
            zoom.pan(dx: d.x, dy: d.y, fit: fitRect)
            setNeedsLayout()
        }
    }

    /** Three fingers scroll, in trackpad mode. */
    @objc private func threePan(_ g: UIPanGestureRecognizer) {
        let d = g.translation(in: self)
        g.setTranslation(.zero, in: self)
        if !touchMode { scroll(d) }
    }

    /** Natural scrolling: the content follows the fingers. */
    private func scroll(_ d: CGPoint) {
        session?.wheel(dx: -d.x * 2, dy: -d.y * 2)
    }

    @objc private func pinch(_ g: UIPinchGestureRecognizer) {
        switch g.state {
        case .began, .changed:
            let at = g.location(in: self)
            zoom.pinch(by: g.scale, aroundX: at.x, aroundY: at.y, fit: fitRect)
            g.scale = 1
            setNeedsLayout()
        default:
            break
        }
    }

    @objc private func hover(_ g: UIHoverGestureRecognizer) {
        if g.state == .began || g.state == .changed { place(g.location(in: self)) }
    }

    @objc private func wheel(_ g: UIPanGestureRecognizer) {
        let d = g.translation(in: self)
        g.setTranslation(.zero, in: self)
        session?.wheel(dx: -d.x, dy: -d.y)
    }

    // A trackpad's or mouse's buttons: indirect pointer touches.
    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        if let t = touches.first, t.type == .indirectPointer, !locked {
            place(t.location(in: self))
            session?.button(event?.buttonMask.contains(.secondary) == true ? .right : .left, down: true, x: pointer.x, y: pointer.y)
            pointerButton = event?.buttonMask.contains(.secondary) == true ? .right : .left
            return
        }
        super.touchesBegan(touches, with: event)
    }

    private var pointerButton: InputPacket.Button?

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        if let t = touches.first, t.type == .indirectPointer {
            place(t.location(in: self))
            return
        }
        super.touchesMoved(touches, with: event)
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        if let t = touches.first, t.type == .indirectPointer, let b = pointerButton {
            place(t.location(in: self))
            session?.button(b, down: false, x: pointer.x, y: pointer.y)
            pointerButton = nil
            return
        }
        super.touchesEnded(touches, with: event)
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        if let b = pointerButton {
            session?.button(b, down: false, x: pointer.x, y: pointer.y)
            pointerButton = nil
        }
        super.touchesCancelled(touches, with: event)
    }

    // MARK: - Keyboard

    override var canBecomeFirstResponder: Bool { true }

    /** The system's three-finger gestures (undo, redo, the edit menu) would take the scrolling: off here. */
    override var editingInteractionConfiguration: UIEditingInteractionConfiguration { .none }

    /** The on-screen keyboard shows only when asked; a hardware keyboard always works. */
    override var inputView: UIView? { softKeyboard ? nil : hiddenKeyboard }

    override var inputAccessoryView: UIView? { softKeyboard ? accessory : nil }

    func setSoftKeyboard(_ on: Bool) {
        guard on != softKeyboard else { return }
        softKeyboard = on
        if isFirstResponder { reloadInputViews() } else { becomeFirstResponder() }
    }

    override func didMoveToWindow() {
        super.didMoveToWindow()
        if window != nil {
            DispatchQueue.main.async { self.becomeFirstResponder() }
            NotificationCenter.default.addObserver(self, selector: #selector(keyboardHid), name: UIResponder.keyboardDidHideNotification, object: nil)
        } else {
            releaseAll()
        }
    }

    @objc private func keyboardHid() {
        if softKeyboard {
            softKeyboard = false
            onKeyboardHidden?()
        }
    }

    var hasText: Bool { true }

    func insertText(_ text: String) {
        if locked { return }
        if text == "\n" {
            press("Enter")
            return
        }
        // With a latched modifier, a letter or digit is a shortcut (Ctrl+C), by position.
        if latched != 0, text.count == 1, let c = text.lowercased().first, let code = Self.code(of: c) {
            press(code)
            return
        }
        session?.input(InputPacket.textEvent(text))
    }

    func deleteBackward() { if !locked { press("Backspace") } }

    /** Presses and lets go of a key, with the latched modifiers held around it. */
    func press(_ code: String) {
        var codes: [String] = []
        if latched & InputPacket.modCtrl != 0 { codes.append("ControlLeft") }
        if latched & InputPacket.modAlt != 0 { codes.append("AltLeft") }
        if latched & InputPacket.modMeta != 0 { codes.append("MetaLeft") }
        if latched & InputPacket.modShift != 0 { codes.append("ShiftLeft") }
        codes.append(code)
        for p in Shortcut(id: "", codes: codes).packets() { session?.input(p) }
        if latched != 0 {
            latched = 0
            onLatch?(0)
        }
    }

    func shortcut(_ s: Shortcut) {
        for p in s.packets() { session?.input(p) }
    }

    func toggleLatch(_ mod: UInt8) {
        latched ^= mod
        onLatch?(latched)
    }

    static func code(of c: Character) -> String? {
        if let a = c.asciiValue {
            if a >= 97 && a <= 122 { return "Key" + String(UnicodeScalar(a - 32)) }
            if a >= 48 && a <= 57 { return "Digit\(a - 48)" }
        }
        return nil
    }

    override func pressesBegan(_ presses: Set<UIPress>, with event: UIPressesEvent?) {
        if locked { return }
        var rest = Set<UIPress>()
        for p in presses {
            guard let key = p.key, let code = Keys.code(hidUsage: key.keyCode.rawValue), let k = Keys.keycode(code) else {
                rest.insert(p)
                continue
            }
            held.insert(k)
            session?.input(InputPacket.keyEvent(k, down: true, mods: Self.mods(key.modifierFlags)))
        }
        if !rest.isEmpty { super.pressesBegan(rest, with: event) }
    }

    override func pressesEnded(_ presses: Set<UIPress>, with event: UIPressesEvent?) {
        var rest = Set<UIPress>()
        for p in presses {
            guard let key = p.key, let code = Keys.code(hidUsage: key.keyCode.rawValue), let k = Keys.keycode(code) else {
                rest.insert(p)
                continue
            }
            // Only what went down here goes up (not a key held while locked).
            if held.remove(k) != nil {
                session?.input(InputPacket.keyEvent(k, down: false, mods: Self.mods(key.modifierFlags)))
            }
        }
        if !rest.isEmpty { super.pressesEnded(rest, with: event) }
    }

    override func pressesCancelled(_ presses: Set<UIPress>, with event: UIPressesEvent?) {
        releaseAll()
        super.pressesCancelled(presses, with: event)
    }

    /** Lets go of every key and button held (the app went away, the session ended). */
    func releaseAll() {
        if !held.isEmpty || dragging || pointerButton != nil {
            session?.input(InputPacket.releaseAllEvent())
        }
        held = []
        dragging = false
        pointerButton = nil
    }

    static func mods(_ f: UIKeyModifierFlags) -> UInt8 {
        var m: UInt8 = 0
        if f.contains(.shift) { m |= InputPacket.modShift }
        if f.contains(.control) { m |= InputPacket.modCtrl }
        if f.contains(.alternate) { m |= InputPacket.modAlt }
        if f.contains(.command) { m |= InputPacket.modMeta }
        if f.contains(.alphaShift) { m |= InputPacket.modCaps }
        return m
    }
}

/** RemoteScreenView in SwiftUI. */
struct RemoteScreen: UIViewRepresentable {
    @ObservedObject var model: SessionModel

    func makeUIView(context: Context) -> RemoteScreenView {
        let v = RemoteScreenView(frame: .zero)
        v.session = model.session
        v.onKeyboardHidden = { [weak model] in model?.keyboard = false }
        model.onHostPointer = { [weak v] x, y, visible in v?.hostPointer(x: x, y: y, visible: visible) }
        context.coordinator.view = v
        return v
    }

    func updateUIView(_ v: RemoteScreenView, context: Context) {
        v.show(model.video)
        v.touchMode = model.touchMode
        v.locked = model.locked
        v.setCursor(model.cursor, pointsWidth: model.displays.first { $0.id == model.currentDisplay }?.pointsWidth ?? 0)
        v.mac = model.hostOS == "darwin"
        v.setSoftKeyboard(model.keyboard)
        if model.status.isOver { v.releaseAll() }
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    final class Coordinator {
        weak var view: RemoteScreenView?
    }
}

/** The keys the on-screen keyboard lacks, over it. Ctrl, Alt and Cmd latch for the next key. */
final class KeyBar: UIInputView {
    private weak var screen: RemoteScreenView?
    private var modButtons: [(UInt8, UIButton)] = []
    private let stack = UIStackView()

    init(screen: RemoteScreenView) {
        self.screen = screen
        super.init(frame: CGRect(x: 0, y: 0, width: 0, height: 48), inputViewStyle: .keyboard)
        allowsSelfSizing = true
        let scroll = UIScrollView()
        scroll.showsHorizontalScrollIndicator = false
        scroll.translatesAutoresizingMaskIntoConstraints = false
        addSubview(scroll)
        stack.axis = .horizontal
        stack.spacing = 6
        stack.translatesAutoresizingMaskIntoConstraints = false
        scroll.addSubview(stack)
        NSLayoutConstraint.activate([
            scroll.leadingAnchor.constraint(equalTo: leadingAnchor),
            scroll.trailingAnchor.constraint(equalTo: trailingAnchor),
            scroll.topAnchor.constraint(equalTo: topAnchor),
            scroll.bottomAnchor.constraint(equalTo: bottomAnchor),
            heightAnchor.constraint(equalToConstant: 48),
            stack.leadingAnchor.constraint(equalTo: scroll.contentLayoutGuide.leadingAnchor, constant: 8),
            stack.trailingAnchor.constraint(equalTo: scroll.contentLayoutGuide.trailingAnchor, constant: -8),
            stack.centerYAnchor.constraint(equalTo: scroll.centerYAnchor),
            stack.heightAnchor.constraint(equalToConstant: 36),
        ])
        relabel(mac: false)
        screen.onLatch = { [weak self] m in self?.showLatch(m) }
    }

    required init?(coder: NSCoder) { fatalError("not used") }

    func relabel(mac: Bool) {
        for v in stack.arrangedSubviews { v.removeFromSuperview() }
        modButtons = []
        add("esc") { $0.press("Escape") }
        add("tab") { $0.press("Tab") }
        mod("ctrl", InputPacket.modCtrl)
        mod(mac ? "⌥" : "alt", InputPacket.modAlt)
        mod(mac ? "⌘" : "win", InputPacket.modMeta)
        mod("⇧", InputPacket.modShift)
        add("←") { $0.press("ArrowLeft") }
        add("↑") { $0.press("ArrowUp") }
        add("↓") { $0.press("ArrowDown") }
        add("→") { $0.press("ArrowRight") }
        add(mac ? "⌦" : "del") { $0.press("Delete") }
        add("home") { $0.press("Home") }
        add("end") { $0.press("End") }
        add("pg↑") { $0.press("PageUp") }
        add("pg↓") { $0.press("PageDown") }
        for i in 1...12 { add("F\(i)") { $0.press("F\(i)") } }
    }

    private func button(_ title: String) -> UIButton {
        var c = UIButton.Configuration.gray()
        c.title = title
        c.cornerStyle = .medium
        c.contentInsets = NSDirectionalEdgeInsets(top: 4, leading: 10, bottom: 4, trailing: 10)
        let b = UIButton(configuration: c)
        b.titleLabel?.font = .systemFont(ofSize: 15, weight: .medium)
        stack.addArrangedSubview(b)
        return b
    }

    private func add(_ title: String, _ fn: @escaping (RemoteScreenView) -> Void) {
        let b = button(title)
        b.addAction(UIAction { [weak self] _ in
            if let s = self?.screen { fn(s) }
        }, for: .touchUpInside)
    }

    private func mod(_ title: String, _ m: UInt8) {
        let b = button(title)
        b.addAction(UIAction { [weak self] _ in self?.screen?.toggleLatch(m) }, for: .touchUpInside)
        modButtons.append((m, b))
    }

    private func showLatch(_ latched: UInt8) {
        for (m, b) in modButtons {
            var c = latched & m != 0 ? UIButton.Configuration.filled() : UIButton.Configuration.gray()
            c.title = b.configuration?.title
            c.cornerStyle = .medium
            c.contentInsets = NSDirectionalEdgeInsets(top: 4, leading: 10, bottom: 4, trailing: 10)
            b.configuration = c
        }
    }
}
