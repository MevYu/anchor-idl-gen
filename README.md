# anchor-idl-gen

`anchor-idl-gen` generates Go bindings from an [Anchor](https://www.anchor-lang.com/)
IDL JSON file. Any program whose IDL is published can be bound in minutes — no
reflection, no manual byte wrangling. Generated code targets
[`github.com/cielu/solana-go`](https://github.com/cielu/solana-go).

## Installation

```sh
go install github.com/MevYu/anchor-idl-gen@latest
```

Or build from source:

```sh
git clone https://github.com/MevYu/anchor-idl-gen
cd anchor-idl-gen
go build -o anchor-idl-gen .
```

## Usage

```
anchor-idl-gen -idl <path/to/program.json> -out <output/dir> [-package <name>]
```

| Flag | Required | Description |
|------|----------|-------------|
| `-idl` | yes | Path to the Anchor IDL JSON file |
| `-out` | yes | Output directory (created if absent) |
| `-package` | no | Go package name; defaults to `metadata.name` from the IDL |

### Example

```sh
anchor-idl-gen \
  -idl testdata/vault.json \
  -out programs/vault \
  -package vault
```

This produces six files in `programs/vault/`:

```
programs/vault/
  types.go         # struct and enum type definitions
  accounts.go      # account discriminators + MatchAccount helper
  instructions.go  # typed instruction builders (Borsh-encoded Data())
  errors.go        # error code sentinels + ErrorByCode lookup
  events.go        # event structs with discriminators
  pda.go           # PDA derivation helpers (DeriveXxx functions)
```

## IDL format

`anchor-idl-gen` consumes the standard Anchor IDL JSON format (spec `0.1.0`):

```json
{
  "address": "Vau1t...",
  "metadata": { "name": "vault", "version": "0.1.0", "spec": "0.1.0" },
  "instructions": [...],
  "accounts":     [...],
  "types":        [...],
  "events":       [...],
  "errors":       [...]
}
```

See [`testdata/minimal.json`](testdata/minimal.json) and
[`testdata/vault.json`](testdata/vault.json) for annotated examples.

## Generated output

### `instructions.go`

Each instruction becomes a typed struct that implements `solana.Instruction`:

```go
// Generated for the vault IDL:
type DepositInstruction struct {
    Vault     solana.PublicKey // writable
    Depositor solana.PublicKey // signer, writable
    Amount    uint64
}

func (ix *DepositInstruction) ProgramID() solana.PublicKey { return ProgramAddress }
func (ix *DepositInstruction) Accounts() []*solana.AccountMeta { ... }
func (ix *DepositInstruction) Data() ([]byte, error) { ... } // 8-byte discriminator + Borsh args
```

Use it directly with `solana.NewMessage`:

```go
ix := &vault.DepositInstruction{
    Vault:     vaultKey,
    Depositor: signerKey,
    Amount:    1_000_000,
}
msg, err := solana.NewMessage(signerKey, []solana.Instruction{ix}, recentBlockhash)
tx := solana.NewTransaction(*msg)
```

### `accounts.go`

```go
// 8-byte Anchor discriminator for on-chain account matching.
var VaultStateDiscriminator = [8]byte{0xe4, 0xc4, 0x52, 0xa5, ...}

// MatchAccount returns the account name if data starts with its discriminator.
func MatchAccount(data []byte) string { ... }
```

### `errors.go`

```go
var ErrInsufficientFunds = errors.New("InsufficientFunds: Insufficient funds in vault")
var ErrUnauthorized     = errors.New("Unauthorized: Not the vault authority")

// ErrorByCode maps an on-chain program error code to a typed sentinel.
func ErrorByCode(code uint32) error { ... }
```

### `events.go`

```go
type DepositedEvent struct {
    Depositor solana.PublicKey
    Amount    uint64
}

var DepositedDiscriminator = [8]byte{0x6f, 0x8d, 0x1a, 0x2d, ...}
```

### `pda.go`

Accounts with a `pda` definition in the IDL get a typed derivation helper:

```go
// DeriveInitializeVault derives the vault PDA for the Initialize instruction.
func DeriveInitializeVault(authority solana.PublicKey) (solana.PublicKey, uint8, error) {
    return solana.FindProgramAddress(
        [][]byte{[]byte("vault"), authority[:]},
        ProgramAddress,
    )
}
```

### `types.go`

IDL `types` entries become Go structs (Borsh-compatible field order) or `uint8`
enums. Example:

```go
type VaultState struct {
    Authority solana.PublicKey
    Balance   uint64
    Bump      uint8
}

type Direction uint8

const (
    DirectionUp   Direction = 0
    DirectionDown Direction = 1
)
```

## Type mapping

| Anchor IDL type | Go type |
|-----------------|---------|
| `u8` | `uint8` |
| `u16` | `uint16` |
| `u32` | `uint32` |
| `u64` | `uint64` |
| `i8` | `int8` |
| `i16` | `int16` |
| `i32` | `int32` |
| `i64` | `int64` |
| `bool` | `bool` |
| `string` | `string` |
| `pubkey` | `solana.PublicKey` |
| `bytes` | `[]byte` |
| `{ vec: T }` | `[]T` |
| `{ array: [T, N] }` | `[N]T` |
| `{ option: T }` | `*T` |
| `{ defined: { name: X } }` | `X` (generated struct/enum) |

## Testing

The generator ships with a golden-file test suite. After modifying a generator,
regenerate the golden files and commit them:

```sh
# regenerate
go run . -idl testdata/minimal.json -out testdata/golden -package counter

# run tests
go test ./...
```

## License

Apache-2.0. See [LICENSE](LICENSE).
