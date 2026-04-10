# jsonr

**jsonr** unmarshals JSON into Go values using dot-notation paths on struct tags, while streaming tokens from [`encoding/json/jsontext`](https://pkg.go.dev/encoding/json/jsontext). You describe only the fields you need; everything else can be skipped without loading the whole document into memory.

## Status

jsonr is in active development. Public APIs and behavior may change without notice; there is no commitment to stability or backward compatibility yet. The software is provided as-is, without warranties or guarantees — use it only after your own evaluation.

## Requirements

- Go 1.26.1 or later (see `go.mod`)

## Install

```bash
go get github.com/exnihilocode/jsonr
```

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

Paths are written in `jsonr` struct tags, and the same string form is used for **`Inline` / `InlineDecode`** (with the extra rules noted at the end of this section).

### How paths are split

- Segments are separated by **`.`** (dot).
- Dots **inside** a `[...]` bracket segment do **not** split the path—only brackets outside any `[`…`]` pair act as segment boundaries. For example, `y.[a,b]` is two segments: `y` and `[a,b]`.
- Leading and trailing space around a path or individual segments is trimmed. Empty paths, empty segments, or empty names inside a bracket list are invalid.

### Literal keys and array indices

Segments that are not `*` and not a `[...]` bracket list are **literal path pieces**:

- Under a JSON **object**, the segment selects the member whose **name** equals the segment text (for example `address.street`). This includes names that look like numbers, such as `"0"` or `"123"`, when those are the actual object keys.
- Under a JSON **array**, the segment must be a non‑negative decimal integer in normal form (`0`, `1`, …): it selects that **index** (`0` is the first element). For example `items.0.id`.

Together, dotted paths walk nested objects and arrays (for example `address.coordinates.latitude`).

### Wildcard (`*`)

- The segment `*` matches **each** value in a JSON **array** (by index, in order) **or** each **member** of a JSON **object** (each key’s value, in object key order). The next segments apply under each of those values.
- The destination field must be a **slice** (or pointer to slice). Values are **appended** in traversal order.
- If a particular element or member does not contain the rest of the path (missing key, wrong shape, etc.), that entry is **skipped** with **no** placeholder and **no** error—collection is best-effort.

### Multi-field selection (`[key,key,...]`)

- A segment wrapped in `[` and `]` lists several **object key names** separated by commas. Whitespace around names is ignored; empty names are invalid.
- For **`Unmarshal` into a struct**, this expands the tag into multiple concrete paths (one per name). The field type must be **`map[string]T`** with **string** keys so each selected key can be stored under its JSON name. **`[]T`**, **`[N]T`**, and other shapes are not supported for multi-field struct tags in the current implementation.
- Bracket segments name **keys only**. They are **not** slice ranges: syntax like `[0:5]` is **not** supported.

### Quick reference

| Form | Meaning |
|------|--------|
| `a.b.c` | Nested object keys. |
| `items.0.id` | Array index then key (`0` = first element). |
| `people.*.name` | Wildcard: collect `name` from every array element or object member under `people`; **slice** field; misses skipped. |
| `obj.[a,b]` | Multi-field: read keys `a` and `b` into **`map[string]T`**. Keys only, not ranges. |

### `Inline` and `InlineDecode`

These APIs use the same segment vocabulary, with a few extra constraints:

- A **multi-field** bracket segment must be the **last** segment in the path (for example `x.y.*.[a,b]` with destination `[]map[string]T`).
- **Wildcard** paths require a **slice** destination type; **multi-field** paths require **`map[string]T`** (see the API section above).
- Inline path parsing rejects **index-range** syntax inside bracket keys (a `:` in a multi-field name), since ranges are not implemented.

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

For more tag examples, see [docs/path_examples.md](docs/path_examples.md).

## License

This project is released under the [MIT License](LICENSE.md).

## Documentation

Run `go doc -all` in this package or see [doc.go](doc.go) for the full package overview.
