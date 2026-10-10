// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Ports of the test stack, apart from the developer's own (signalhub 8090,
// web 5180) and from go-link's test stack (8191, 5191-5194, 7391).
export const PORTS = { signal: 8291, web: 5291 };

/** The test signalhub, as the host and the browser reach it. */
export const SIGNAL_URL = `ws://127.0.0.1:${PORTS.signal}/ws`;
