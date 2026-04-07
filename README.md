# jsonr

**jsonr** unmarshals JSON into Go values using dot-notation paths on struct tags, while streaming tokens from [`encoding/json/jsontext`](https://pkg.go.dev/encoding/json/jsontext). You describe only the fields you need; everything else can be skipped without loading the whole document into memory.

## Requirements

- Go 1.26.1 or later (see `go.mod`)

## Install

```bash
go get <module/path>
```

Use the import path from your published `go.mod` (for example `github.com/you/jsonr`). This repository currently declares `module jsonr`; replace `<module/path>` accordingly when you release.

## Quick start

```go
import (
	"log"
	"strings"

	"jsonr"
)

var v struct {
	Name      string  `jsonr:"name"`
	Latitude  float64 `jsonr:"address.coordinates.latitude"`
	Longitude float64 `jsonr:"address.coordinates.longitude"`
}
input := `{"name":"North","address":{"coordinates":{"latitude":12.5,"longitude":-3.0}},"noise":{"x":1}}`
if err := jsonr.Unmarshal(strings.NewReader(input), &v); err != nil {
	log.Fatal(err)
}
// v.Name == "North", v.Latitude, v.Longitude set; "noise" was skipped at token scope
```

## API

- **`Unmarshal(r io.Reader, v any) error`** — decode into `v` from a reader. `v` must be a non-nil pointer to a struct, map, slice, or array. For structs, only exported fields with a `jsonr` tag are filled; other fields are left untouched.

- **`UnmarshalDecode(dec *jsontext.Decoder, v any) error`** — same as `Unmarshal`, but you supply a [`jsontext.Decoder`](https://pkg.go.dev/encoding/json/jsontext#Decoder). Use this when you already have a decoder or want to read multiple top-level JSON values from one stream.

- **`Inline[T any](r io.Reader, path string) (T, error)`** — read **one** value at `path` from the reader. The next token must start a JSON **object** or **array**. Path syntax matches struct tags (`*`, numeric indices, and a terminal `y.[a,b]` multi-field segment). Wildcards require a **slice** destination; bracket segments require **`map[string]T`**. Struct destinations are rejected except **[time.Time]**. On success, that value is consumed; trailing siblings stay in the stream.

- **`InlineDecode[T any](dec *jsontext.Decoder, path string) (T, error)`** — same as **Inline**, but reads from an existing **`jsontext.Decoder`** at the start of an object or array.

### Time values (`time.Time`)

`time.Time` fields (and slices or pointers of them) decode from JSON **strings**, **numbers**, or **`null`**. `null` yields the zero time, or a nil `*time.Time`.

A JSON **number** is treated as **Unix time in seconds** (UTC), same as [`time.Unix`](https://pkg.go.dev/time#Unix). The decoder uses the number’s **integer** value; any fractional part is **truncated toward zero** (matching [`jsontext.Token.Int`](https://pkg.go.dev/encoding/json/jsontext#Token.Int)).

Accepted **string** forms include **RFC3339** / **RFC3339Nano** (typical ISO 8601 with offset or `Z`), **date-time without zone** in UTC (`YYYY-MM-DDTHH:MM:SS` or space instead of `T`), **date only** `YYYY-MM-DD` at **00:00 UTC**, and **time only** `HH:MM:SS` or `HH:MM` in **UTC** (calendar date parts are the zero date; see package `time`). Surrounding ASCII space is trimmed. A numeric value sent as a **quoted string** is parsed with the string rules above, not as Unix seconds.

## Path syntax (`jsonr` struct tags)

| Form | Meaning |
|------|--------|
| `a.b.c` | Walk nested objects by key. |
| `items.0.id` | Index into a JSON array (`0` is the first element). |
| `people.*.name` | For each array element, resolve `name`; append to a **slice** field (elements without the path are skipped). |
| `obj.[a,b]` | Read multiple keys from one object into **`map[string]T`**. Not supported for slice/array destinations. Brackets list key names only, not ranges like `[0:5]`. |

**Inline** supports the same path forms as the table above in one path string; multi-field brackets must be the **last** segment (for example `x.y.*.[a,b]` into `[]map[string]T`).

## Examples

### Wildcard into a slice

```go
import (
	"strings"

	"jsonr"
)

var v struct {
	Names []string `jsonr:"people.*.name"`
}
input := `{"people":[{"name":"a"},{"skip":true},{"name":"b"}]}`
_ = jsonr.Unmarshal(strings.NewReader(input), &v)
// v.Names == []string{"a","b"}
```

### Multi-field into a map

```go
import (
	"strings"

	"jsonr"
)

var v struct {
	X int             `jsonr:"x"`
	Y map[string]int  `jsonr:"y.[a,b]"`
}
input := `{"x":1,"y":{"a":2,"b":3,"c":4}}`
_ = jsonr.Unmarshal(strings.NewReader(input), &v)
// v.Y has keys "a" and "b" only
```

### Inline decode from a decoder

```go
import (
	"encoding/json/jsontext"
	"log"
	"strings"

	"jsonr"
)

dec := jsontext.NewDecoder(strings.NewReader(`{"address":{"coordinates":{"latitude":1.5,"longitude":2.5}}}`))
lat, err := jsonr.InlineDecode[float64](dec, "address.coordinates.latitude")
if err != nil {
	log.Fatal(err)
}
// dec is now positioned after the latitude value; you could InlineDecode another field or continue manually
_ = lat
```

### Inline wildcard and multi-field

```go
dec := jsontext.NewDecoder(strings.NewReader(`{"x":{"y":[{"a":1,"c":9},{"a":2,"b":3}]}}`))
rows, err := jsonr.InlineDecode[[]map[string]int](dec, "x.y.*.[a,b]")
if err != nil {
	log.Fatal(err)
}
// rows == []map[string]int{{"a":1}, {"a":2,"b":3}} — first object has no "b", so that map omits it
_ = rows
```

## Design notes

- **Streaming:** Matching paths are decoded; other subtrees are skipped via the decoder without materializing them as large Go values.
- **Structs:** Tag-only selection—there is no fallback to `encoding/json` field names for untagged struct fields in this mode.
- **Not supported:** Slice-range syntax in paths; multi-field tags into `[]T` / `[N]T` (use `map[string]T` for multi-key object picks).

For more tag examples, see [docs/path_examples.md](docs/path_examples.md). Some examples there describe roadmap or alternate shapes; bracket syntax is **keys only**, and multi-field tags require a **map** destination in the current implementation.

## Documentation

Run `go doc -all` in this package or see [doc.go](doc.go) for the full package overview.
