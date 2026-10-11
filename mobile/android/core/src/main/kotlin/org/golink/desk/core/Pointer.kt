// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import kotlin.math.max
import kotlin.math.min
import kotlin.math.sqrt

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
data class Pointer(var x: Double = 0.5, var y: Double = 0.5) {
    /**
     * Moves by a finger's slide of dx, dy screen points over dt seconds,
     * on a picture shown pictureWidth × pictureHeight points large (with
     * the zoom). Slow slides are precise (one point of finger is one point
     * of picture); fast ones go further, up to 3 times.
     */
    fun slide(dx: Double, dy: Double, dt: Double, pictureWidth: Double, pictureHeight: Double) {
        if (pictureWidth <= 0 || pictureHeight <= 0) return
        val speed = sqrt(dx * dx + dy * dy) / max(dt, 0.001) // points a second
        val g = gain(speed)
        x = (x + dx * g / pictureWidth).coerceIn(0.0, 1.0)
        y = (y + dy * g / pictureHeight).coerceIn(0.0, 1.0)
    }

    /** Puts the pointer at a point of the view, given where the picture is in it. */
    fun place(viewX: Double, viewY: Double, picture: Rect) {
        if (picture.width <= 0 || picture.height <= 0) return
        x = ((viewX - picture.x) / picture.width).coerceIn(0.0, 1.0)
        y = ((viewY - picture.y) / picture.height).coerceIn(0.0, 1.0)
    }

    /** The pointer's point in the view. */
    fun inView(picture: Rect): Pair<Double, Double> = (picture.x + x * picture.width) to (picture.y + y * picture.height)

    companion object {
        /** The acceleration: 1 up to 300 points a second, growing to 3 at 1500. */
        fun gain(speed: Double): Double = if (speed <= 300) 1.0 else min(3.0, 1 + (speed - 300) / 600)
    }
}

/** A rectangle, in points. */
data class Rect(val x: Double, val y: Double, val width: Double, val height: Double) {
    /** A picture of size w × h fitted into this rectangle, centered, keeping its shape. */
    fun fit(w: Double, h: Double): Rect {
        if (w <= 0 || h <= 0 || width <= 0 || height <= 0) return this
        val s = min(width / w, height / h)
        return Rect(x + (width - w * s) / 2, y + (height - h * s) / 2, w * s, h * s)
    }
}

/**
 * The picture's zoom and pan: 1 shows the whole picture; up to 5 times
 * larger. While zoomed, the view follows the pointer so it never leaves
 * the screen.
 */
class Zoom {
    var scale = 1.0
        private set

    /** The zoomed picture's top left corner, relative to the fitted picture's. */
    var offsetX = 0.0
        private set
    var offsetY = 0.0
        private set

    /** The zoomed picture's rectangle, for a fitted picture fit. */
    fun apply(fit: Rect): Rect = Rect(fit.x - offsetX, fit.y - offsetY, fit.width * scale, fit.height * scale)

    /** Pinches by factor around a point of the view, keeping that point still. */
    fun pinch(factor: Double, ax: Double, ay: Double, fit: Rect) {
        val next = (scale * factor).coerceIn(1.0, MAX_SCALE)
        val f = next / scale
        // The point under the fingers stays where it is.
        offsetX = (offsetX + (ax - fit.x)) * f - (ax - fit.x)
        offsetY = (offsetY + (ay - fit.y)) * f - (ay - fit.y)
        scale = next
        clamp(fit)
    }

    /** Pans the zoomed picture by dx, dy points. */
    fun pan(dx: Double, dy: Double, fit: Rect) {
        offsetX -= dx
        offsetY -= dy
        clamp(fit)
    }

    /** Moves the view so the point at px, py (in the view) is inside it, margin points from its edges. Returns whether it moved. */
    fun follow(px: Double, py: Double, view: Rect, fit: Rect, margin: Double = 40.0): Boolean {
        if (scale <= 1) return false
        val ox = offsetX
        val oy = offsetY
        if (px < view.x + margin) offsetX -= view.x + margin - px
        if (px > view.x + view.width - margin) offsetX += px - (view.x + view.width - margin)
        if (py < view.y + margin) offsetY -= view.y + margin - py
        if (py > view.y + view.height - margin) offsetY += py - (view.y + view.height - margin)
        clamp(fit)
        return ox != offsetX || oy != offsetY
    }

    fun reset() {
        scale = 1.0
        offsetX = 0.0
        offsetY = 0.0
    }

    private fun clamp(fit: Rect) {
        offsetX = offsetX.coerceIn(0.0, max(0.0, fit.width * (scale - 1)))
        offsetY = offsetY.coerceIn(0.0, max(0.0, fit.height * (scale - 1)))
        if (scale <= 1) {
            offsetX = 0.0
            offsetY = 0.0
        }
    }

    companion object {
        const val MAX_SCALE = 5.0
    }
}
