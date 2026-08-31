# PC compatibility — plan

Status: **plan only, nothing implemented.** Research done against
`er_bak/ER0000.sl2` (read-only), SaveForge at revision `053afaf2`, and
`ER_research/save_info/save-architecture.md`.

## The headline: no new cryptography is needed

The regulation layer is **byte-for-byte the same pipeline on both platforms**:

```text
USER_DATA011  ->  " GER" header  ->  AES-256-CBC  ->  DCX/ZSTD  ->  BND4  ->  PARAM
```

The AES-256-CBC regulation key in `pipeline_crypto.go` is **byte-identical** to
SaveForge's, and its own comment already says "same for PC and PS4". Verified by
comparison. `ShopLineupParam` lives in exactly the same place.

**Everything that makes this editor what it is — the schema, the shop patching,
the recompression, the reopen verification — is untouched by this work.** Only
the outer container differs.

## What actually differs

| | PC (`ER0000.sl2`) | Decrypted PS4 (`memory.dat`) |
|---|---|---|
| Container header | `0x300`, magic `BND4` | `0x70`, magic `CB 01 9C 2C` |
| Entry framing | 12 × (`0x10` MD5 + data) | 12 × data |
| `USER_DATA011` offset | `0x19603C0` | `0x19003A0` |
| Total size | `0x1BA03D0` | `0x1BA0080` |

Confirmed directly against the backup: `BND4` magic, 12 entries, the MD5 of every
entry body matches its 16-byte prefix, `USER_DATA011` begins with ` GER`, and the
declared layout consumes the file to the exact byte.

## Settled: the `.sl2` is plaintext BND4

This was the open question. It is now answered from source rather than assumed.

**SaveForge's own SL2 binary-format specification states it outright:**

> None of the 3 reference editors encrypts/decrypts the `.sl2` file. The `.sl2`
> file on PC is normally saved as plaintext BND4.

The AES-128-CBC it implements is a **Steam layer** — Steam on Windows desktop
encrypts the file before writing it to disk, and **on Steam Deck it does not**.
That is why SaveForge can *produce* an encrypted PC save while refusing to *open*
one: the encryption is not the game's, it is Steam's.

Corroborated three ways:

1. **SoulsFormats** (`SL2Decryptor.cs`) ships save keys for **DS2, Scholar and
   DS3 only — none for Elden Ring**. Its DS2/DS3 per-entry layout is
   `[MD5][IV][ciphertext]`; Elden Ring kept the MD5 and dropped the entry
   encryption.
2. **The fixture agrees.** In `er_bak/ER0000.sl2` every entry's MD5 matches its
   body, and `USER_DATA011`'s body begins with the plaintext ` GER` magic. An IV
   cannot coincidentally read as ` GER`.
3. Community documentation describes per-`USERDATA` AES for the Souls line, which
   is the DS3 behaviour SoulsFormats encodes — not Elden Ring's.

**Consequence: the primary path needs no outer cryptography at all.** Detection
by leading magic is enough, exactly as SaveForge does it: leading `BND4` →
plaintext; `BND4` only after decryption → Steam-encrypted.

### The Steam-encrypted variant, deferred

Whole-file AES-128-CBC, IV = the file's first 16 bytes, key
`99 AD 2D 50 ED F2 FB 01 C5 F3 EC 3A 2B CA B6 9D`. It is one function each way
with SaveForge's `crypto.go` as reference, but it is **not** implemented in the
first pass: it cannot be tested without a Windows-Steam save, and shipping
untested crypto that rewrites a save file is precisely the wrong trade. The
loader refuses such files with a clear message instead.

### A correction to the upstream spec

SaveForge's spec lists the `USER_DATA011` MD5 at `0x1F003B0`. That is a typo —
`0x19003A0 + 0x60010 = 0x19603B0`, which is where the digest actually verifies
and where ` GER` actually appears. Our value is the measured one.

## The change, in three parts

### 1. Platform detection — by magic only

```go
type Platform int   // PlatformPS4, PlatformPC

func classify(data []byte) (Platform, error)
```

Rules, taken from SaveForge's hard-won design:

- `BND4` prefix → PC. `CB 01 9C 2C` prefix → PS4. Anything else → **refuse**.
- **Never** infer from the file extension. `.sl2` and `.dat` are conventions, not
  guarantees.
- **Never** decrypt-and-guess. Treating "AES → BND4" as PC would let a PS-origin
  file be written back in the wrong container.
- **Never** convert between platforms. Out of scope, and a silent conversion is
  the worst possible failure here.

### 2. `userData11Bounds` becomes platform-aware

Today it is hardcoded:

```go
off := ps4HeaderSize + numSlots*slotSize + userdata10Size   // 0x70 + 10*0x280000 + 0x60000
```

PC needs the MD5 framing:

```go
off := pcHeaderSize                              // 0x300
    + numSlots*(md5Size+slotSize)                // 10 × (0x10 + 0x280000)
    + (md5Size + userdata10Size)                 // 0x10 + 0x60000
    + md5Size                                    // UD11's own digest
```

`Regulation` gains a `Platform` field so the write path knows what it opened.
The ` GER` magic check stays for both — it is a real integrity check, and the
current error message ("only PS4 saves are supported") is what has to change.

### 3. Recompute the `USER_DATA011` MD5 on write

The only genuinely new write logic. `encryptAndSplice` already copies the whole
file and overwrites just the IV + ciphertext; for PC it must then write
`MD5(UD11 body)` into the 16 bytes at `UD11Off - 0x10`.

Miss this and the game rejects the save — or worse, silently repairs it.

## What must not change

- The PS4 path stays byte-identical. Existing fixtures must still reproduce their
  recorded hashes.
- Distinct output path, atomic write, reopen-and-verify. Unchanged.
- The regulation version guard. A 1.16.1 save opened under 1.17 is still refused,
  on both platforms.

## Verification

1. **Round-trip, no edits.** Open the PC backup, write with an empty edit set,
   assert the output is byte-identical to the input. This alone catches offsets,
   MD5, and IV reuse.
2. **PS4 regression.** Existing tests unchanged and still green.
3. **One shop edit, PC.** Apply, reopen, confirm the row changed and that the
   only differing bytes are inside `USER_DATA011` plus its 16-byte digest.
4. **Cross-platform refusal.** A PS4 file must not open as PC, and vice versa;
   assert on the error, not on a log line.
5. **Truncated and hand-corrupted inputs** are refused, not partially written.
6. **In game.** The real test. A PC save edited by the tool must load, show the
   new stock, and survive a save/reload.

Only step 6 needs the game; 1–5 are offline and should exist first.

## Effort

Small, and much smaller than it looked before the container was examined. The
inner pipeline is shared, the key is shared, and the reference implementation is
sitting in `external/`. The work is a detection function, an offset calculation,
one MD5 write, and the tests above.

The risk is not complexity — it is **writing a file back in the wrong container
shape**, which is why detection refuses ambiguity instead of guessing.

## Open questions

- Encrypted vs plaintext `.sl2` — the blocker above.
- Does the PC save's Steam ID binding matter for a regulation-only edit? It
  should not, since we touch nothing in `USER_DATA010`, but worth confirming that
  an edited save still loads on the account that owns it.
- The backup's regulation reads version `11601000`, older than our 1.17 baseline.
  A current PC save is needed for the in-game test, or the version guard will
  correctly refuse it.
