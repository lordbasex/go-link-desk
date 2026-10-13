# signalhub proposals

go-link Desk uses [signalhub](https://github.com/lordbasex/signalhub) exactly as it is. When Desk would benefit from something signalhub does not do, it is written here as a proposal, never patched into this repository or worked around by bending the protocol.

A proposal must be **generic**: useful to any application on signalhub, not shaped around go-link Desk. Each one says:

- **Need:** what an application cannot do today, with a concrete case.
- **Proposal:** the smallest change to the protocol or the server's settings.
- **Compatibility:** what happens to existing clients (go-link, go-link Desk, others).
- **Alternatives:** what an application can do without it, and why that is worse.

## Open

None yet.

## Notes for a future proposal

- **Removing a guest.** signalhub has no way for an owner to remove a guest from its session. go-link Desk ignores a rejected viewer until it leaves, which works. A generic `kick {peer}` for owners would let an owner free that guest's place right away. Not proposed yet: there is no case where ignoring is not enough.
- **Relays run by others.** signalhub signs every TURN credential with one `TURN_SECRET`, so every relay it hands out must hold that secret, and a relay that holds it can mint credentials for all the others. A volunteer relay in another region (the connection test invites them, [network.md](network.md)) would need its own secret: `TURN_URLS` entries with a secret each, and `hello` choosing the relay nearest the client. Not proposed yet: the project's own relays share one secret today.
