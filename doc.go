/*
Package jsonr streams JSON and decodes only the values described by dot-notation
paths in struct tags ("jsonr"), without buffering entire documents. Uninterested
subtrees are skipped at the token level using [encoding/json/jsontext].

# Unmarshal

Unmarshal fills exported fields that carry a jsonr tag. The destination must be
a non-nil pointer to a struct, map, slice, or fixed-size array. For a struct, the
document must be a top-level JSON object or null. For a map, the document must be
an object. For a slice or array, the document must be a JSON array. Fields without
a jsonr tag are ignored when unmarshaling into a struct.

# Path syntax (jsonr struct tags)

  - Dot-separated keys traverse nested objects (for example address.coordinates.latitude).

  - A numeric segment selects an array element (for example items.0.id).

  - An asterisk walks every array element and collects values; the destination
    field must be a slice (or pointer to slice). Elements missing the rest of the
    path are skipped with no placeholder entry.

  - A bracket segment lists multiple keys under one object (for example obj.[a,b]);
    the tag expands to several paths, so the destination field must be map[string]T
    with string keys. Slice and array destinations are not supported for multi-field
    tags. Bracket syntax names keys only, not slice ranges like [0:5].

# Time values

Fields of type [time.Time] decode from a JSON string, a JSON number, or null. JSON
null sets the field to the zero time. For [time] pointer fields, null sets the
pointer to nil.

A JSON number is interpreted as **Unix time in seconds** (see [time.Unix]) and
decodes to UTC. The numeric token’s integer value is used; any fractional part is
truncated toward zero (consistent with [encoding/json/jsontext.Token.Int]). JSON strings use the following layouts, tried in order:
RFC3339 with optional fractional seconds, RFC3339, date-time without a zone in UTC
(layouts "2006-01-02T15:04:05" and "2006-01-02 15:04:05"), date-only "2006-01-02"
at midnight UTC, then time-only "15:04:05" or "15:04" in UTC. For time-only strings,
missing calendar fields use the zero date in UTC (as with [time.ParseInLocation]
in UTC). Leading and trailing ASCII space is trimmed before parsing strings.
Quoted numeric strings are parsed as strings, not as Unix seconds.

# Inline

Inline decodes one JSON value selected by path from the reader's current position.
Path uses the same dot notation as jsonr struct tags, including wildcard segments
("*") and multi-field bracket segments that must appear last (e.g. "y.[a,b]").
Wildcard paths require a slice destination; multi-field paths require map[string]T.
Elements missing the rest of the path under a wildcard are skipped with no placeholder.

The destination type must not be a struct other than [time.Time]. The document root
must be a JSON object or array. After a successful call, the reader is advanced past
the decoded value. Remaining tokens in the document (sibling keys, trailing values)
are left unconsumed.

# InlineDecode

InlineDecode decodes a single value from a [encoding/json/jsontext.Decoder] already
positioned at the start of a JSON object or array. See the Inline section for path
syntax and constraints. After success the decoder has consumed the selected value;
remaining input is left for the caller.

# Relationship to encoding/json

jsonr is a companion to encoding/json: it uses the standard library's JSON token
decoder for streaming and does not replace full Unmarshal of arbitrary untagged types.
*/
package jsonr
