// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** The input packets against protocol/vectors/input.json, the bytes the host, the website and the iOS app test too. */
class InputPacketTest {
    @Test
    fun vectors() {
        val root = parseObject(repoFile("protocol/vectors/input.json").readText())!!
        val packets = root["packets"].arr()
        assertTrue(packets.size > 10)
        for (v in packets) {
            val o = v.obj()
            val name = o["name"].str()
            val p = o["packet"].obj()
            val want = o["hex"].str().replace(" ", "")
            fun n(k: String) = p[k].num().toInt()
            val bytes = when (p["type"].str()) {
                "abs" -> InputPacket.pointer(n("seq"), n("display"), n("x"), n("y"))
                "rel" -> InputPacket.relative(n("seq"), n("dx"), n("dy"))
                "button" -> InputPacket.buttonEvent(InputPacket.Button.entries.first { it.wire == n("button") }, p["down"].isTrue(), n("display"), n("x"), n("y"))
                "wheel" -> InputPacket.wheelEvent(n("dx"), n("dy"), n("mode") == 1)
                "key" -> InputPacket.keyEvent(n("keycode"), p["down"].isTrue(), p["repeat"].isTrue(), n("mods"))
                "text" -> InputPacket.textEvent(p["text"].str(100_000))
                "release_all" -> InputPacket.releaseAllEvent()
                else -> error("unknown packet $name")
            }
            assertEquals(name, want, hex(bytes))
        }
    }

    @Test
    fun textIsCutWithoutSplittingACharacter() {
        val b = InputPacket.textEvent("a".repeat(1023) + "ñ")
        assertEquals(1 + 1023, b.size)
        assertEquals(65535, InputPacket.coord(1.0))
        assertEquals(0, InputPacket.coord(-2.0))
    }

    @Test
    fun keysByPosition() {
        assertEquals("KeyA", Keys.code(29))
        assertEquals("KeyZ", Keys.code(54))
        assertEquals("Digit0", Keys.code(7))
        assertEquals("Digit9", Keys.code(16))
        assertEquals("F1", Keys.code(131))
        assertEquals("F12", Keys.code(142))
        assertEquals("Numpad1", Keys.code(145))
        assertEquals("MetaLeft", Keys.code(117))
        assertEquals("Escape", Keys.code(111))
        assertEquals(1, Keys.keycode("KeyA"))
        assertNull(Keys.keycode("Unidentified"))
        assertNull(Keys.keycode("NoSuchKey"))
        // Every Android key the map names is in the table.
        for (k in 0..300) Keys.code(k)?.let { assertNotNull("$k -> $it", Keys.keycode(it)) }
        assertEquals(InputPacket.MOD_SHIFT or InputPacket.MOD_CTRL, Keys.mods(0x1 or 0x1000))
        assertEquals("KeyC", Keys.codeOf('c'))
        assertEquals("Digit5", Keys.codeOf('5'))
        assertNull(Keys.codeOf('ñ'))
    }

    @Test
    fun shortcutPressesInOrderAndReleasesInReverse() {
        val s = Shortcut("x", listOf("MetaLeft", "ShiftLeft", "Digit5"))
        val meta = Keys.keycode("MetaLeft")!!
        val shift = Keys.keycode("ShiftLeft")!!
        val five = Keys.keycode("Digit5")!!
        val ms = InputPacket.MOD_META or InputPacket.MOD_SHIFT
        assertEquals(
            listOf(
                InputPacket.keyEvent(meta, true, mods = InputPacket.MOD_META),
                InputPacket.keyEvent(shift, true, mods = ms),
                InputPacket.keyEvent(five, true, mods = ms),
                InputPacket.keyEvent(five, false, mods = ms),
                InputPacket.keyEvent(shift, false, mods = InputPacket.MOD_META),
                InputPacket.keyEvent(meta, false, mods = 0),
            ).map(::hex),
            s.packets().map(::hex),
        )
        assertTrue(Shortcut.forSystem("darwin").any { it.id == "spotlight" })
        assertTrue(Shortcut.forSystem("windows").any { it.id == "ctrl_alt_del" })
    }
}

class PointerTest {
    @Test
    fun slowSlidesArePreciseAndFastOnesGoFurther() {
        val p = Pointer()
        p.slide(10.0, 0.0, 0.1, 1000.0, 500.0)
        assertEquals(0.51, p.x, 1e-9)
        val q = Pointer()
        q.slide(100.0, 0.0, 0.05, 1000.0, 500.0) // 2000 points a second
        assertEquals(0.8, q.x, 1e-9)
        q.slide(1000.0, 1000.0, 0.01, 1000.0, 500.0)
        assertEquals(1.0, q.x, 0.0)
        assertEquals(1.0, q.y, 0.0)
    }

    @Test
    fun placeAndFit() {
        val fit = Rect(0.0, 0.0, 1000.0, 1000.0).fit(1920.0, 1080.0)
        assertEquals(1000.0, fit.width, 1e-9)
        assertEquals(562.5, fit.height, 1e-9)
        assertEquals(218.75, fit.y, 1e-9)
        val p = Pointer()
        p.place(500.0, fit.y, fit)
        assertEquals(0.5, p.x, 1e-9)
        assertEquals(0.0, p.y, 1e-9)
    }

    @Test
    fun zoomKeepsThePointUnderTheFingers() {
        val fit = Rect(0.0, 0.0, 1000.0, 500.0)
        val z = Zoom()
        z.pinch(2.0, 250.0, 125.0, fit)
        val r = z.apply(fit)
        assertEquals(2000.0, r.width, 0.0)
        assertEquals(0.25, (250 - r.x) / r.width, 1e-9)
        assertEquals(0.25, (125 - r.y) / r.height, 1e-9)
        z.pinch(100.0, 0.0, 0.0, fit)
        assertEquals(Zoom.MAX_SCALE, z.scale, 0.0)
        z.pan(1e6, 1e6, fit)
        assertEquals(0.0, z.offsetX, 0.0)
        z.pinch(0.01, 0.0, 0.0, fit)
        assertEquals(fit, z.apply(fit))
    }
}

class RecentsTest {
    @Test
    fun newestFirstWithoutRepeats() {
        val a = Recent("111111111", "A", "darwin", 1)
        val b = Recent("222222222", "B", "windows", 2)
        var list = Recents.add(a, emptyList())
        list = Recents.add(b, list)
        list = Recents.add(a, list)
        assertEquals(listOf("111111111", "222222222"), list.map { it.code })
        for (i in 0 until 20) list = Recents.add(Recent("%09d".format(i), "", "", 0), list)
        assertEquals(Recents.MAX, list.size)
        assertEquals(list, Recents.decode(Recents.encode(list)))
        assertEquals(emptyList<Recent>(), Recents.decode("not json"))
        assertEquals("915355636", Recents.digits("915 355 636"))
        assertNull(Recents.digits("915 355 63"))
        assertNull(Recents.digits("https://x/915355636"))
    }

    @Test
    fun statsRates() {
        val m = StatsMeter()
        assertEquals("direct", m.next(StatsMeter.Sample(0.0, 0, 0, 0, 0, localType = "host", remoteType = "prflx")).path)
        val r = m.next(StatsMeter.Sample(2000.0, 120, 1_000_000, 90, 10, localType = "relay", remoteType = "host"))
        assertEquals(60.0, r.fps, 0.0)
        assertEquals(4000, r.kbps)
        assertEquals(10.0, r.lossPct, 0.0)
        assertEquals("relay", r.path)
    }
}
