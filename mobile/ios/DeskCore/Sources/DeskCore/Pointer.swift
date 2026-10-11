// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

/**
 * Where the computer's pointer is, and how a finger moves it.
 *
 * Trackpad mode (the default, as on a laptop): a finger sliding anywhere
 * moves the pointer by how far it slid, faster for a quick swipe, and a
 * tap clicks where the pointer is. Touch mode: the pointer jumps to the
 * finger, and a tap clicks there.
 *
 * Positions are 0..1 over the computer's display.
 */
public struct Pointer: Equatable, Sendable {
    public var x = 0.5
    public var y = 0.5

    public init(x: Double = 0.5, y: Double = 0.5) {
        self.x = x
        self.y = y
    }

    /**
     * Moves by a finger's slide of dx, dy screen points over dt seconds,
     * on a picture shown pictureWidth × pictureHeight points large (with
     * the zoom). Slow slides are precise (one point of finger is one point
     * of picture); fast ones go further, up to 3 times.
     */
    public mutating func slide(dx: Double, dy: Double, dt: Double, pictureWidth: Double, pictureHeight: Double) {
        guard pictureWidth > 0, pictureHeight > 0 else { return }
        let speed = (dx * dx + dy * dy).squareRoot() / max(dt, 0.001) // points a second
        let gain = Pointer.gain(speed)
        x = (x + dx * gain / pictureWidth).clamped(0, 1)
        y = (y + dy * gain / pictureHeight).clamped(0, 1)
    }

    /** The acceleration: 1 up to 300 points a second, growing to 3 at 1500. */
    public static func gain(_ speed: Double) -> Double {
        if speed <= 300 { return 1 }
        return min(3, 1 + (speed - 300) / 600)
    }

    /** Puts the pointer at a point of the view, given where the picture is in it. */
    public mutating func place(viewX: Double, viewY: Double, picture: Rect) {
        guard picture.width > 0, picture.height > 0 else { return }
        x = ((viewX - picture.x) / picture.width).clamped(0, 1)
        y = ((viewY - picture.y) / picture.height).clamped(0, 1)
    }

    /** The pointer's point in the view. */
    public func inView(_ picture: Rect) -> (x: Double, y: Double) {
        (picture.x + x * picture.width, picture.y + y * picture.height)
    }
}

/** A rectangle, in points. */
public struct Rect: Equatable, Sendable {
    public var x, y, width, height: Double

    public init(x: Double, y: Double, width: Double, height: Double) {
        self.x = x
        self.y = y
        self.width = width
        self.height = height
    }

    /** A picture of size w × h fitted into this rectangle, centered, keeping its shape. */
    public func fit(_ w: Double, _ h: Double) -> Rect {
        guard w > 0, h > 0, width > 0, height > 0 else { return self }
        let s = min(width / w, height / h)
        return Rect(x: x + (width - w * s) / 2, y: y + (height - h * s) / 2, width: w * s, height: h * s)
    }
}

/**
 * The picture's zoom and pan: 1 shows the whole picture; up to 5 times
 * larger. While zoomed, the view follows the pointer so it never leaves
 * the screen.
 */
public struct Zoom: Equatable, Sendable {
    public private(set) var scale = 1.0
    /** The zoomed picture's top left corner, relative to the fitted picture's. */
    public private(set) var offsetX = 0.0
    public private(set) var offsetY = 0.0

    public static let maxScale = 5.0

    public init() {}

    /** The zoomed picture's rectangle, for a fitted picture fit. */
    public func apply(_ fit: Rect) -> Rect {
        Rect(x: fit.x - offsetX, y: fit.y - offsetY, width: fit.width * scale, height: fit.height * scale)
    }

    /** Pinches by factor around a point of the view, keeping that point still. */
    public mutating func pinch(by factor: Double, aroundX ax: Double, aroundY ay: Double, fit: Rect) {
        let next = (scale * factor).clamped(1, Zoom.maxScale)
        let f = next / scale
        // The point under the fingers stays where it is.
        offsetX = (offsetX + (ax - fit.x)) * f - (ax - fit.x)
        offsetY = (offsetY + (ay - fit.y)) * f - (ay - fit.y)
        scale = next
        clamp(fit)
    }

    /** Pans the zoomed picture by dx, dy points. */
    public mutating func pan(dx: Double, dy: Double, fit: Rect) {
        offsetX -= dx
        offsetY -= dy
        clamp(fit)
    }

    /** Moves the view so the point at px, py (in the view) is inside it, margin points from its edges. */
    public mutating func follow(px: Double, py: Double, view: Rect, fit: Rect, margin: Double = 40) {
        guard scale > 1 else { return }
        if px < view.x + margin { offsetX -= view.x + margin - px }
        if px > view.x + view.width - margin { offsetX += px - (view.x + view.width - margin) }
        if py < view.y + margin { offsetY -= view.y + margin - py }
        if py > view.y + view.height - margin { offsetY += py - (view.y + view.height - margin) }
        clamp(fit)
    }

    public mutating func reset() {
        scale = 1
        offsetX = 0
        offsetY = 0
    }

    private mutating func clamp(_ fit: Rect) {
        let maxX = fit.width * (scale - 1)
        let maxY = fit.height * (scale - 1)
        offsetX = offsetX.clamped(0, max(0, maxX))
        offsetY = offsetY.clamped(0, max(0, maxY))
        if scale <= 1 { offsetX = 0; offsetY = 0 }
    }
}
