// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.ui

import android.annotation.SuppressLint
import android.content.Context
import android.graphics.BitmapFactory
import android.graphics.SurfaceTexture
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.text.InputType
import android.view.HapticFeedbackConstants
import android.view.InputDevice
import android.view.KeyCharacterMap
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.TextureView
import android.view.View
import android.view.ViewConfiguration
import android.view.inputmethod.BaseInputConnection
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputConnection
import android.view.inputmethod.InputMethodManager
import android.widget.FrameLayout
import android.widget.ImageView
import org.golink.desk.core.CursorShape
import org.golink.desk.core.DeskSession
import org.golink.desk.core.InputPacket
import org.golink.desk.core.Keys
import org.golink.desk.core.Pointer
import org.golink.desk.core.Rect
import org.golink.desk.core.Shortcut
import org.golink.desk.core.Zoom
import org.webrtc.EglBase
import org.webrtc.EglRenderer
import org.webrtc.GlRectDrawer
import org.webrtc.VideoFrame
import org.webrtc.VideoSink
import org.webrtc.VideoTrack
import kotlin.math.abs
import kotlin.math.hypot
import kotlin.math.max

/**
 * The computer's screen: its picture (decoded by the hardware), its
 * pointer, and the fingers, mouse and keyboard of this device. The
 * Android counterpart of the iOS app's RemoteScreenView.
 *
 * Fingers, trackpad mode (the default): one finger slides the pointer, a
 * tap clicks, a long press then a slide drags; two fingers: a tap is a
 * right click, a pinch zooms and, zoomed in, a slide moves the view;
 * three fingers scroll. Touch mode: the pointer jumps to the finger, a
 * tap clicks there, and two fingers scroll.
 *
 * A mouse or trackpad moves the pointer directly, its buttons click and
 * its wheel scrolls. A hardware keyboard's keys go by position (the same
 * place on the computer's keyboard); the on-screen keyboard sends the text
 * typed.
 */
@SuppressLint("ViewConstructor")
class RemoteScreenView(context: Context, egl: EglBase.Context) : FrameLayout(context) {
    var session: DeskSession? = null
    var touchMode = false

    /** Locked: nothing is sent and the view stays still (to watch). */
    var locked = false
        set(v) {
            if (v && !field) releaseAll()
            field = v
        }

    /** Modifiers latched from the bar (Ctrl, Alt, Cmd), for the next key. */
    var latched = 0
        private set
    var onLatch: ((Int) -> Unit)? = null

    private val main = Handler(Looper.getMainLooper())
    private val density = resources.displayMetrics.density
    private val slop = ViewConfiguration.get(context).scaledTouchSlop.toFloat()
    private val texture = TextureView(context)
    private val cursorView = ImageView(context)
    private val renderer = EglRenderer("desk-picture")
    private var track: VideoTrack? = null
    private var videoW = 0
    private var videoH = 0
    private val pointer = Pointer()
    private val zoom = Zoom()
    private var cursorShape: CursorShape? = null
    private var pointsWidth = 0
    private var lastWant = 0L
    private val held = HashSet<Int>()

    private val sink = VideoSink { frame: VideoFrame ->
        val w = frame.rotatedWidth
        val h = frame.rotatedHeight
        if (w != videoW || h != videoH) {
            main.post {
                videoW = w
                videoH = h
                requestLayout()
            }
        }
        renderer.onFrame(frame)
    }

    init {
        setBackgroundColor(android.graphics.Color.BLACK)
        clipChildren = true
        isFocusable = true
        isFocusableInTouchMode = true
        renderer.init(egl, EglBase.CONFIG_PLAIN, GlRectDrawer())
        texture.surfaceTextureListener = object : TextureView.SurfaceTextureListener {
            override fun onSurfaceTextureAvailable(st: SurfaceTexture, w: Int, h: Int) = renderer.createEglSurface(st)

            override fun onSurfaceTextureSizeChanged(st: SurfaceTexture, w: Int, h: Int) = Unit

            override fun onSurfaceTextureDestroyed(st: SurfaceTexture): Boolean {
                val done = java.util.concurrent.CountDownLatch(1)
                renderer.releaseEglSurface { done.countDown() }
                done.await(1, java.util.concurrent.TimeUnit.SECONDS)
                return true
            }

            override fun onSurfaceTextureUpdated(st: SurfaceTexture) = Unit
        }
        addView(texture)
        cursorView.visibility = View.INVISIBLE
        addView(cursorView, LayoutParams(0, 0))
    }

    // Picture

    fun show(t: VideoTrack?) {
        if (t === track) return
        track?.let { runCatching { it.removeSink(sink) } }
        track = t
        if (t == null) renderer.clearImage() else runCatching { t.addSink(sink) }
    }

    fun release() {
        show(null)
        renderer.release()
    }

    /** Where the whole picture is, fitted in the view (no zoom), in pixels. */
    private val fitRect: Rect
        get() {
            val view = Rect(0.0, 0.0, width.toDouble(), height.toDouble())
            return if (videoW > 0) view.fit(videoW.toDouble(), videoH.toDouble()) else view
        }

    /** Where the picture is now, with the zoom. */
    private val pictureRect: Rect get() = zoom.apply(fitRect)

    override fun onLayout(changed: Boolean, l: Int, t: Int, r: Int, b: Int) {
        val f = fitRect
        texture.layout(f.x.toInt(), f.y.toInt(), (f.x + f.width).toInt(), (f.y + f.height).toInt())
        applyZoom()
        placeCursor()
        if (changed) wantSize()
    }

    /** The zoom as the picture's transform: it stays the fitted picture's size underneath. */
    private fun applyZoom() {
        texture.pivotX = 0f
        texture.pivotY = 0f
        texture.scaleX = zoom.scale.toFloat()
        texture.scaleY = zoom.scale.toFloat()
        texture.translationX = (-zoom.offsetX).toFloat()
        texture.translationY = (-zoom.offsetY).toFloat()
    }

    /** Tells the computer the size the picture is shown at, in pixels (once the view settles). */
    private fun wantSize() {
        if (width <= 0 || height <= 0) return
        val want = (width.toLong() shl 32) or height.toLong()
        if (want == lastWant) return
        lastWant = want
        main.removeCallbacks(sendWant)
        main.postDelayed(sendWant, 400)
    }

    private val sendWant = Runnable { session?.setVideoWant(width, height) }

    // Pointer

    fun setCursor(shape: CursorShape?, pointsWidth: Int) {
        this.pointsWidth = pointsWidth
        if (shape === cursorShape) return
        cursorShape = shape
        cursorView.setImageBitmap(shape?.let { BitmapFactory.decodeByteArray(it.png, 0, it.png.size) })
        placeCursor()
    }

    /** The computer moved its pointer by itself (or another viewer did). */
    fun hostPointer(x: Double, y: Double, visible: Boolean) {
        if (dragging || fingers > 0) return
        pointer.x = x
        pointer.y = y
        cursorView.alpha = if (visible) 1f else 0f
        placeCursor()
    }

    private fun placeCursor() {
        val r = pictureRect
        val shape = cursorShape
        if (videoW <= 0 || shape == null || cursorView.drawable == null) {
            cursorView.visibility = View.INVISIBLE
            return
        }
        val (px, py) = pointer.inView(r)
        // The image is sized in the computer's points: the same share of the picture here.
        val perPoint = if (pointsWidth > 0) r.width / pointsWidth else r.width / max(videoW, 1)
        var w = 18.0 * density
        var h = 18.0 * density
        var hx = 0.0
        var hy = 0.0
        if (shape.width > 0 && shape.height > 0) {
            w = shape.width * perPoint
            h = shape.height * perPoint
            val pxPerPoint = if (shape.pixels > 0) shape.pixels / shape.width else 1.0
            hx = shape.hotX / pxPerPoint * perPoint
            hy = shape.hotY / pxPerPoint * perPoint
        }
        // Never tiny on a phone.
        val grow = max(1.0, 14 * density / max(w, 1.0))
        w *= grow
        h *= grow
        hx *= grow
        hy *= grow
        cursorView.visibility = View.VISIBLE
        val left = (px - hx).toInt()
        val top = (py - hy).toInt()
        cursorView.layout(left, top, left + w.toInt(), top + h.toInt())
    }

    private fun movePointer() {
        session?.pointer(pointer.x, pointer.y)
        // Zoomed in, the view follows the pointer.
        val (px, py) = pointer.inView(pictureRect)
        if (zoom.follow(px, py, Rect(0.0, 0.0, width.toDouble(), height.toDouble()), fitRect, 40.0 * density)) applyZoom()
        placeCursor()
    }

    private fun place(x: Float, y: Float) {
        pointer.place(x.toDouble(), y.toDouble(), pictureRect)
        movePointer()
    }

    private fun click(b: InputPacket.Button) {
        session?.button(b, true, pointer.x, pointer.y)
        session?.button(b, false, pointer.x, pointer.y)
    }

    // Fingers

    private var fingers = 0
    private var maxFingers = 0
    private var downAt = 0L
    private var startX = 0f
    private var startY = 0f
    private var lastX = 0f
    private var lastY = 0f
    private var lastAt = 0L
    private var moved = false
    private var dragging = false
    private var spread = 0f
    private var centerX = 0f
    private var centerY = 0f
    private var travel = 0f
    private var mouseButtons = 0

    private val longPress = Runnable {
        if (fingers == 1 && maxFingers == 1 && !moved && !locked) {
            if (touchMode) place(startX, startY)
            dragging = true
            performHapticFeedback(HapticFeedbackConstants.LONG_PRESS)
            session?.button(InputPacket.Button.LEFT, true, pointer.x, pointer.y)
        }
    }

    /** The middle of the fingers down, and how far apart they are. */
    private fun measure(e: MotionEvent) {
        var sx = 0f
        var sy = 0f
        val n = e.pointerCount
        for (i in 0 until n) {
            sx += e.getX(i)
            sy += e.getY(i)
        }
        centerX = sx / n
        centerY = sy / n
        spread = if (n >= 2) hypot(e.getX(0) - e.getX(1), e.getY(0) - e.getY(1)) else 0f
    }

    @SuppressLint("ClickableViewAccessibility")
    override fun onTouchEvent(e: MotionEvent): Boolean {
        if (e.isFromSource(InputDevice.SOURCE_MOUSE) || e.getToolType(0) == MotionEvent.TOOL_TYPE_MOUSE) return mouse(e)
        if (locked) return true
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                requestFocus()
                fingers = 1
                maxFingers = 1
                downAt = SystemClock.uptimeMillis()
                startX = e.x
                startY = e.y
                lastX = e.x
                lastY = e.y
                lastAt = downAt
                moved = false
                travel = 0f
                main.postDelayed(longPress, 400)
            }
            MotionEvent.ACTION_POINTER_DOWN -> {
                fingers = e.pointerCount
                maxFingers = max(maxFingers, fingers)
                main.removeCallbacks(longPress)
                measure(e)
            }
            MotionEvent.ACTION_MOVE -> {
                if (fingers == 1 && maxFingers == 1) oneFinger(e) else if (fingers >= 2) moreFingers(e)
            }
            MotionEvent.ACTION_POINTER_UP -> {
                fingers = e.pointerCount - 1
                // The rest go on from where they are, with no jump.
                val keep = (0 until e.pointerCount).filter { it != e.actionIndex }
                if (keep.isNotEmpty()) {
                    centerX = keep.map { e.getX(it) }.average().toFloat()
                    centerY = keep.map { e.getY(it) }.average().toFloat()
                    spread = if (keep.size >= 2) hypot(e.getX(keep[0]) - e.getX(keep[1]), e.getY(keep[0]) - e.getY(keep[1])) else 0f
                }
            }
            MotionEvent.ACTION_UP -> {
                main.removeCallbacks(longPress)
                val quick = SystemClock.uptimeMillis() - downAt
                when {
                    dragging -> {
                        dragging = false
                        session?.button(InputPacket.Button.LEFT, false, pointer.x, pointer.y)
                    }
                    maxFingers == 1 && !moved && quick < 350 -> {
                        if (touchMode) place(e.x, e.y)
                        click(InputPacket.Button.LEFT)
                    }
                    maxFingers == 2 && travel < slop * 2 && quick < 400 -> {
                        // Right click (where the fingers are, in touch mode).
                        if (touchMode) place(centerX, centerY)
                        click(InputPacket.Button.RIGHT)
                    }
                }
                fingers = 0
            }
            MotionEvent.ACTION_CANCEL -> {
                main.removeCallbacks(longPress)
                if (dragging) {
                    dragging = false
                    session?.button(InputPacket.Button.LEFT, false, pointer.x, pointer.y)
                }
                fingers = 0
            }
        }
        return true
    }

    private fun oneFinger(e: MotionEvent) {
        if (!moved && hypot(e.x - startX, e.y - startY) > slop) {
            moved = true
            if (!dragging) main.removeCallbacks(longPress)
        }
        if (!moved && !dragging) return
        val now = SystemClock.uptimeMillis()
        if (touchMode) {
            place(e.x, e.y)
        } else {
            val r = pictureRect
            pointer.slide(
                ((e.x - lastX) / density).toDouble(),
                ((e.y - lastY) / density).toDouble(),
                (now - lastAt) / 1000.0,
                r.width / density,
                r.height / density,
            )
            movePointer()
        }
        lastX = e.x
        lastY = e.y
        lastAt = now
    }

    private fun moreFingers(e: MotionEvent) {
        val px = centerX
        val py = centerY
        val ps = spread
        measure(e)
        val dx = centerX - px
        val dy = centerY - py
        travel += hypot(dx, dy) + abs(spread - ps)
        if (e.pointerCount >= 3) {
            // Three fingers scroll, in trackpad mode.
            if (!touchMode) scroll(dx, dy)
            return
        }
        if (ps > 0 && spread > 0 && travel > slop) {
            zoom.pinch((spread / ps).toDouble(), centerX.toDouble(), centerY.toDouble(), fitRect)
        }
        if (touchMode) {
            if (travel > slop) scroll(dx, dy)
        } else if (zoom.scale > 1) {
            zoom.pan(dx.toDouble(), dy.toDouble(), fitRect)
        }
        applyZoom()
        placeCursor()
    }

    /** Natural scrolling: the content follows the fingers. */
    private fun scroll(dx: Float, dy: Float) {
        session?.wheel(-dx / density * 2.0, -dy / density * 2.0)
    }

    // A mouse or trackpad

    private fun mouse(e: MotionEvent): Boolean {
        if (locked) return true
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE, MotionEvent.ACTION_HOVER_MOVE -> place(e.x, e.y)
            MotionEvent.ACTION_BUTTON_PRESS -> {
                place(e.x, e.y)
                mouseButton(e.actionButton)?.let {
                    mouseButtons = mouseButtons or e.actionButton
                    session?.button(it, true, pointer.x, pointer.y)
                }
            }
            MotionEvent.ACTION_BUTTON_RELEASE -> {
                place(e.x, e.y)
                mouseButton(e.actionButton)?.let {
                    mouseButtons = mouseButtons and e.actionButton.inv()
                    session?.button(it, false, pointer.x, pointer.y)
                }
            }
            MotionEvent.ACTION_SCROLL -> session?.wheel(
                -e.getAxisValue(MotionEvent.AXIS_HSCROLL) * 80.0,
                -e.getAxisValue(MotionEvent.AXIS_VSCROLL) * 80.0,
            )
        }
        return true
    }

    private fun mouseButton(b: Int): InputPacket.Button? = when (b) {
        MotionEvent.BUTTON_PRIMARY -> InputPacket.Button.LEFT
        MotionEvent.BUTTON_SECONDARY -> InputPacket.Button.RIGHT
        MotionEvent.BUTTON_TERTIARY -> InputPacket.Button.MIDDLE
        MotionEvent.BUTTON_BACK -> InputPacket.Button.BACK
        MotionEvent.BUTTON_FORWARD -> InputPacket.Button.FORWARD
        else -> null
    }

    override fun onGenericMotionEvent(e: MotionEvent): Boolean {
        if (e.isFromSource(InputDevice.SOURCE_CLASS_POINTER)) return mouse(e)
        return super.onGenericMotionEvent(e)
    }

    // Keyboard

    fun setSoftKeyboard(on: Boolean) {
        val imm = context.getSystemService(InputMethodManager::class.java)
        if (on) {
            requestFocus()
            imm.showSoftInput(this, 0)
        } else {
            imm.hideSoftInputFromWindow(windowToken, 0)
        }
    }

    override fun onCheckIsTextEditor(): Boolean = true

    override fun onCreateInputConnection(info: EditorInfo): InputConnection {
        info.inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_VISIBLE_PASSWORD or InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
        info.imeOptions = EditorInfo.IME_FLAG_NO_EXTRACT_UI or EditorInfo.IME_FLAG_NO_FULLSCREEN or EditorInfo.IME_ACTION_NONE
        return Typing()
    }

    /**
     * What the on-screen keyboard types: text as text, its Delete and Enter
     * as keys. A keyboard that composes words (suggestions) is followed by
     * deleting and typing the difference.
     */
    private inner class Typing : BaseInputConnection(this@RemoteScreenView, false) {
        private var composing = ""

        private fun replace(next: String) {
            var same = 0
            while (same < composing.length && same < next.length && composing[same] == next[same]) same++
            repeat(composing.length - same) { press("Backspace") }
            if (next.length > same) type(next.substring(same))
            composing = next
        }

        override fun commitText(text: CharSequence, newCursorPosition: Int): Boolean {
            replace(text.toString())
            composing = ""
            return true
        }

        override fun setComposingText(text: CharSequence, newCursorPosition: Int): Boolean {
            replace(text.toString())
            return true
        }

        override fun finishComposingText(): Boolean {
            composing = ""
            return true
        }

        override fun deleteSurroundingText(beforeLength: Int, afterLength: Int): Boolean {
            if (locked) return true
            repeat(beforeLength.coerceAtMost(64)) { press("Backspace") }
            repeat(afterLength.coerceAtMost(64)) { press("Delete") }
            return true
        }

        override fun sendKeyEvent(event: KeyEvent): Boolean {
            if (event.action == KeyEvent.ACTION_DOWN) {
                when (event.keyCode) {
                    KeyEvent.KEYCODE_DEL -> press("Backspace")
                    KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_NUMPAD_ENTER -> press("Enter")
                    else -> {
                        val code = Keys.code(event.keyCode)
                        if (code != null) {
                            press(code)
                        } else if (event.unicodeChar != 0) {
                            type(String(Character.toChars(event.unicodeChar)))
                        }
                    }
                }
            }
            return true
        }
    }

    /** Typed text; with a latched modifier, a letter or digit is a shortcut (Ctrl+C), by position. */
    private fun type(text: String) {
        if (locked) return
        if (text == "\n") {
            press("Enter")
            return
        }
        if (latched != 0 && text.length == 1) {
            Keys.codeOf(text[0])?.let {
                press(it)
                return
            }
        }
        session?.input(InputPacket.textEvent(text))
    }

    /** Presses and lets go of a key, with the latched modifiers held around it. */
    fun press(code: String) {
        if (locked) return
        val codes = mutableListOf<String>()
        if (latched and InputPacket.MOD_CTRL != 0) codes += "ControlLeft"
        if (latched and InputPacket.MOD_ALT != 0) codes += "AltLeft"
        if (latched and InputPacket.MOD_META != 0) codes += "MetaLeft"
        if (latched and InputPacket.MOD_SHIFT != 0) codes += "ShiftLeft"
        codes += code
        for (p in Shortcut("", codes).packets()) session?.input(p)
        if (latched != 0) {
            latched = 0
            onLatch?.invoke(0)
        }
    }

    fun toggleLatch(mod: Int) {
        latched = latched xor mod
        onLatch?.invoke(latched)
    }

    /** A hardware keyboard's keys, by position; the system's own keys (back, volume) are left alone. */
    override fun dispatchKeyEvent(event: KeyEvent): Boolean {
        val code = Keys.code(event.keyCode)
        val hardware = event.deviceId != KeyCharacterMap.VIRTUAL_KEYBOARD && event.device?.isVirtual == false
        if (!hardware || code == null || event.keyCode in SYSTEM_KEYS) return super.dispatchKeyEvent(event)
        if (locked) return true
        val k = Keys.keycode(code) ?: return super.dispatchKeyEvent(event)
        val mods = Keys.mods(event.metaState)
        when (event.action) {
            KeyEvent.ACTION_DOWN -> {
                held += k
                session?.input(InputPacket.keyEvent(k, true, event.repeatCount > 0, mods))
            }
            KeyEvent.ACTION_UP -> {
                // Only what went down here goes up (not a key held while locked).
                if (held.remove(k)) session?.input(InputPacket.keyEvent(k, false, mods = mods))
            }
        }
        return true
    }

    override fun onFocusChanged(gained: Boolean, direction: Int, previous: android.graphics.Rect?) {
        super.onFocusChanged(gained, direction, previous)
        if (!gained) releaseAll()
    }

    /** Lets go of every key and button held (the app went away, the session ended). */
    fun releaseAll() {
        if (held.isNotEmpty() || dragging || mouseButtons != 0) session?.input(InputPacket.releaseAllEvent())
        held.clear()
        dragging = false
        mouseButtons = 0
    }

    companion object {
        private val SYSTEM_KEYS = setOf(
            KeyEvent.KEYCODE_BACK, KeyEvent.KEYCODE_HOME, KeyEvent.KEYCODE_POWER, KeyEvent.KEYCODE_VOLUME_UP,
            KeyEvent.KEYCODE_VOLUME_DOWN, KeyEvent.KEYCODE_VOLUME_MUTE, KeyEvent.KEYCODE_APP_SWITCH,
        )
    }
}
