# jsonr path examples

This file expands on the tag forms summarized in the [README](../README.md) (“Path syntax”). Behavior described here matches the current implementation.

## How paths are built

- Segments are separated by **`.`** (dot). Dots **inside** a bracket segment `[...]` do **not** split the path (for example `y.[a,b]` is two segments: `y` and `[a,b]`).
- Empty paths, empty segments, or empty names inside a bracket list are invalid.

## Literal keys and array indices

- `jsonr:"address.street"` — Selects the `street` member of the top-level `address` object.
- Under a JSON **object**, each segment matches the member **name** (including names that look like numbers, e.g. `"0"` or `"123"`, when those are the actual keys).
- Under a JSON **array**, use a non‑negative decimal index segment (`0`, `1`, …) for the element position.

Example with an array:

- `jsonr:"addresses.0.street"` — `street` from the first element (index `0`) of the `addresses` array.

## Wildcard (`*`)

The `*` segment walks **every** element of a JSON **array** or **every** member of a JSON **object** at that position; the following segments apply under each value.

- `jsonr:"addresses.*.street"` — Collects `street` from each object in an `addresses` **array**, or from each value if `addresses` is an **object** whose values are objects with `street`. The field must be a **slice** (or pointer to slice). Elements or members that do not contain the rest of the path are **skipped** with no error and no placeholder entry.

## Multi-field selection (`[key,key,...]`)

A bracket segment lists several object keys to read from one object. For **`Unmarshal` into a struct**, the destination must be **`map[string]T`** (string keys). Slice and fixed-array destinations for multi-field tags are **not** supported.

- `jsonr:"address.[street,city,state]"` — Reads only `street`, `city`, and `state` from `address` into a map, e.g. `map[string]string` with keys `"street"`, `"city"`, `"state"`.

Bracket segments name **keys only**; slice-range syntax such as `addresses.[0:5]` is **not** supported.

## `Inline` / `InlineDecode`

Path syntax matches struct tags. Additional rules:

- A multi-field bracket segment must be the **last** segment (e.g. `x.y.*.[a,b]` into `[]map[string]T`).
- Wildcard paths need a slice destination; multi-field paths need `map[string]T` for the selected value type.

## Brief example

Only the name and coordinates matter; the rest of the document can be skipped by the decoder.

### Example JSON

```json
{
    "name": "John Doe",
    "age": 30,
    "email": "john.doe@example.com",
    "address": {
        "street": "123 Main St",
        "city": "Anytown",
        "state": "CA",
        "zip": "12345",
        "country": "USA",
        "coordinates": {
            "latitude": 37.774929,
            "longitude": -122.419416
        }
    }
}
```

### Traditional nested structs

Nested structs (possibly declared inline) mirror the JSON shape:

```go
type Employee struct {
	Name    string  `json:"name"`
	Address Address `json:"address"`
}

type Address struct {
	Coordinates Coordinates `json:"coordinates"`
}

type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
```

### Flat paths (scalars)

One struct with `jsonr` paths avoids intermediate types for leaf values:

```go
type EmployeeCoordinates struct {
	Name      string  `jsonr:"name"`
	Latitude  float64 `jsonr:"address.coordinates.latitude"`
	Longitude float64 `jsonr:"address.coordinates.longitude"`
}
```

### Multi-field into a map

Pick several keys under `coordinates` into one map:

```go
type EmployeeCoordinates struct {
	Name         string             `jsonr:"name"`
	Coordinates  map[string]float64 `jsonr:"address.coordinates.[latitude,longitude]"`
}
```

Equivalent populated values:

```go
var result = EmployeeCoordinates{
	Name: "John Doe",
	Coordinates: map[string]float64{
		"latitude":  37.774929,
		"longitude": -122.419416,
	},
}
```
