#!/usr/bin/env python3
"""On VM 116: real input into the isolated headless session, and PNGs, through its remote.sock.
drive.py OUTDIR step... ; steps: scene:<spec>|name  click:x,y  shot:name  wait:s"""
import os, socket, struct, sys, threading, time, zlib
path = os.path.join(os.environ['XDG_RUNTIME_DIR'], 'lordos', 'remote.sock')
for _ in range(200):
    if os.path.exists(path): break
    time.sleep(0.1)
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM); s.connect(path)
FB = {'w': 0, 'h': 0, 'px': None}
def recvn(n):
    b = b''
    while len(b) < n:
        c = s.recv(n - len(b))
        if not c: raise OSError('closed')
        b += c
    return b
def drain():
    try:
        while True:
            ln, = struct.unpack('<I', recvn(4)); body = recvn(ln); t = body[0]
            if t == 1:
                w, h = struct.unpack('<HH', body[1:5]); FB.update(w=w, h=h, px=bytearray(w * h * 4))
            elif t == 2 and FB['px'] is not None:
                x, y, w, h = struct.unpack('<HHHH', body[1:9]); d = body[9:]
                for r in range(h):
                    o = ((y + r) * FB['w'] + x) * 4
                    FB['px'][o:o + w * 4] = d[r * w * 4:(r + 1) * w * 4]
    except OSError: pass
threading.Thread(target=drain, daemon=True).start()
def msg(t, data=b''): s.sendall(struct.pack('<IB', len(data) + 1, t) + data)
def pump(sec):
    end = time.time() + sec
    while time.time() < end: msg(10); time.sleep(0.1)
def save(name):
    w, h, px = FB['w'], FB['h'], FB['px']
    raw = bytearray()
    for r in range(h):
        raw.append(0); row = px[r * w * 4:(r + 1) * w * 4]
        rgb = bytearray(w * 3); rgb[0::3] = row[2::4]; rgb[1::3] = row[1::4]; rgb[2::3] = row[0::4]
        raw += rgb
    chunk = lambda t, d: struct.pack('>I', len(d)) + t + d + struct.pack('>I', zlib.crc32(t + d) & 0xffffffff)
    png = b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', w, h, 8, 2, 0, 0, 0)) + chunk(b'IDAT', zlib.compress(bytes(raw), 6)) + chunk(b'IEND', b'')
    open(os.path.join(OUT, name + '.png'), 'wb').write(png); print('saved', name, flush=True)
OUT = sys.argv[1]; SCENE = '/home/lord/LordOS-appui/home/scene.txt'; i = 0
msg(32, struct.pack('<BBHI', 1, 0, 60, 1000))   # RP_VIDEO: a viewer, or nothing is composed for us
msg(35, b'\x01')   # RP_VMODE: lossless rectangles once the screen is still
pump(1.5)
for step in sys.argv[2:]:
    op, _, arg = step.partition(':')
    if op == 'wait': pump(float(arg))
    elif op == 'click':
        x, y = map(int, arg.split(',')); msg(11, struct.pack('<hh', x, y)); time.sleep(.08); msg(12, struct.pack('<BB', 0, 1)); time.sleep(.08); msg(12, struct.pack('<BB', 0, 0)); pump(.5)
    elif op == 'move':
        x, y = map(int, arg.split(',')); msg(11, struct.pack('<hh', x, y)); pump(.5)
    elif op == 'shot': pump(0.4); save(arg)
    elif op == 'scene':
        spec, _, name = arg.partition('|'); i += 1
        open(SCENE + '.tmp', 'w').write(spec + ';n=%d' % i); os.replace(SCENE + '.tmp', SCENE)
        pump(1.1)
        if name: save(name)
print('done', flush=True)
