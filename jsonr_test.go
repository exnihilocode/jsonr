package jsonr

import (
	"encoding/json/jsontext"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unsettableTagged has an unexported jsonr-tagged field; reflect cannot set it.
type unsettableTagged struct {
	n int `jsonr:"n"` //nolint:unused // populated via JSON key "n" through reflection only
}

func TestUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "nestedPaths",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					Name      string  `jsonr:"name"`
					Latitude  float64 `jsonr:"address.coordinates.latitude"`
					Longitude float64 `jsonr:"address.coordinates.longitude"`
				}
				const in = `{"name": "North","address": { "coordinates": { "latitude": 12.555555, "longitude": -3.012345 } },"noise": { "x": 1 }}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, "North", v.Name)
				assert.InDelta(t, 12.555555, v.Latitude, 0)
				assert.InDelta(t, -3.012345, v.Longitude, 0)
			},
		},
		{
			name: "wildcardSlice",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					Names []string `jsonr:"people.*.name"`
				}
				const in = `{"people": [{ "name": "a", "skip": true },{ "id": 2 },{ "name": "b" }]}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Len(t, v.Names, 2)
				assert.Equal(t, []string{"a", "b"}, v.Names)
			},
		},
		{
			name: "fullSlice",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					Nums []int `jsonr:"nums"`
				}
				const in = `{"nums":[1,2,3]}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, []int{1, 2, 3}, v.Nums)
			},
		},
		{
			name: "skipSubtree",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					X int `jsonr:"x"`
				}
				in := `{"big":[` + strings.Repeat(`{},`, 200) + `{}], "x": 7}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, 7, v.X)
			},
		},
		{
			name: "subStruct",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					X int `jsonr:"x"`
					Y []*struct {
						A int `jsonr:"a.b.c"`
					} `jsonr:"y"`
				}
				in := `{"x": 1, "y": [{ "a": { "b": { "c": 2 } } },{ "a": { "b": { "c": 3 } } }]}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, 1, v.X)
				require.Len(t, v.Y, 2)
				assert.Equal(t, 2, v.Y[0].A)
				assert.Equal(t, 3, v.Y[1].A)
			},
		},
		{
			name: "topLevelMap",
			run: func(t *testing.T) {
				t.Helper()
				type sub struct {
					A int `jsonr:"object.a"`
					B int `jsonr:"object.b"`
				}
				v := make(map[string]*sub)
				in := `{ "a": { "object": { "a": 1, "b": 2 } }, "b": { "object": { "a": 3, "b": 4 } }}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				require.Contains(t, v, "a")
				require.Contains(t, v, "b")
				assert.Equal(t, 1, v["a"].A)
				assert.Equal(t, 2, v["a"].B)
				assert.Equal(t, 3, v["b"].A)
				assert.Equal(t, 4, v["b"].B)
				assert.Nil(t, v["c"])
			},
		},
		{
			name: "topLevelSlice",
			run: func(t *testing.T) {
				t.Helper()
				type sub struct {
					A int `jsonr:"object.a"`
					B int `jsonr:"object.b"`
				}
				var v []sub
				in := `[{ "object": { "a": 1, "b": 2 } },{ "object": { "a": 3, "b": 4 } }]`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				require.Len(t, v, 2)
				assert.Equal(t, 1, v[0].A)
				assert.Equal(t, 2, v[0].B)
				assert.Equal(t, 3, v[1].A)
				assert.Equal(t, 4, v[1].B)
			},
		},
		{
			name: "topLevelArray",
			run: func(t *testing.T) {
				t.Helper()
				type sub struct {
					A int `jsonr:"object.a"`
					B int `jsonr:"object.b"`
				}
				var v [2]sub
				in := `[{ "object": { "a": 1, "b": 2 } },{ "object": { "a": 3, "b": 4 } }]`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, 1, v[0].A)
				assert.Equal(t, 2, v[0].B)
				assert.Equal(t, 3, v[1].A)
				assert.Equal(t, 4, v[1].B)
			},
		},
		{
			name: "explicitMap",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					X int            `jsonr:"x"`
					Y map[string]int `jsonr:"y.[a,b]"`
				}
				in := `{"x": 1,"y": { "a": 2, "b": 3, "c": 4 }}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, 1, v.X)
				assert.Len(t, v.Y, 2)
				assert.Equal(t, 2, v.Y["a"])
				assert.Equal(t, 3, v.Y["b"])
			},
		},
		{
			name: "timeFieldRFC3339",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					At time.Time `jsonr:"at"`
				}
				in := `{"at": "2024-07-20T15:00:00Z"}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, time.Date(2024, 7, 20, 15, 0, 0, 0, time.UTC), v.At.UTC())
			},
		},
		{
			name: "timeFieldDateOnly",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					At time.Time `jsonr:"at"`
				}
				in := `{"at": "2024-07-20"}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, time.Date(2024, 7, 20, 0, 0, 0, 0, time.UTC), v.At.UTC())
			},
		},
		{
			name: "timeFieldTimeOnly",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					At time.Time `jsonr:"at"`
				}
				in := `{"at": "08:09"}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, time.Date(0, 1, 1, 8, 9, 0, 0, time.UTC), v.At.UTC())
			},
		},
		{
			name: "timePointerNull",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					At *time.Time `jsonr:"at"`
				}
				prev := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
				v.At = &prev
				in := `{"at": null}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Nil(t, v.At)
			},
		},
		{
			name: "wildcardSliceTime",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					Times []time.Time `jsonr:"items.*.t"`
				}
				in := `{"items":[{"t":"2024-01-01T00:00:00Z"},{"skip":1},{"t":"2024-01-03"}]}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				require.Len(t, v.Times, 2)
				assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), v.Times[0].UTC())
				assert.Equal(t, time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), v.Times[1].UTC())
			},
		},
		{
			name: "timeFieldUnix",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					At time.Time `jsonr:"at"`
				}
				x := time.Now().Truncate(time.Second)
				in := `{"at": ` + strconv.FormatInt(x.Unix(), 10) + `}`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, x, v.At)
				assert.Equal(t, x.Unix(), v.At.Unix())
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestUnmarshal_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "nilV",
			run: func(t *testing.T) {
				t.Helper()
				err := Unmarshal(strings.NewReader("{}"), nil)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "non-nil pointer")
			},
		},
		{
			name: "vNotPointer",
			run: func(t *testing.T) {
				t.Helper()
				err := Unmarshal(strings.NewReader("{}"), struct{}{})
				require.Error(t, err)
				assert.Contains(t, err.Error(), "non-nil pointer")
			},
		},
		{
			name: "unsupportedPointerElem",
			run: func(t *testing.T) {
				t.Helper()
				var n int
				err := Unmarshal(strings.NewReader("0"), &n)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "pointer to struct, map, slice, or array")
			},
		},
		{
			name: "structTopLevelArray",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					X int `jsonr:"x"`
				}
				err := Unmarshal(strings.NewReader(`[]`), &v)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "object")
			},
		},
		{
			name: "structTopLevelString",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					X int `jsonr:"x"`
				}
				err := Unmarshal(strings.NewReader(`"hi"`), &v)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "object")
			},
		},
		{
			name: "invalidPathEmptySegment",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					X int `jsonr:"foo..bar"`
				}
				err := Unmarshal(strings.NewReader(`{}`), &v)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "empty path segment")
			},
		},
		{
			name: "wildcardRequiresSlice",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					S string `jsonr:"a.*.b"`
				}
				err := Unmarshal(strings.NewReader(`{}`), &v)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "wildcard path")
				assert.Contains(t, err.Error(), "slice")
			},
		},
		{
			name: "multiFieldRequiresMap",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					S []string `jsonr:"obj.[a,b]"`
				}
				err := Unmarshal(strings.NewReader(`{}`), &v)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "multi-field path")
				assert.Contains(t, err.Error(), "map")
			},
		},
		{
			name: "tagWhitespaceOnlyPath",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					X int `jsonr:" "`
				}
				err := Unmarshal(strings.NewReader(`{}`), &v)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "empty jsonr path")
			},
		},
		{
			name: "unexportedTaggedField",
			run: func(t *testing.T) {
				t.Helper()
				var v unsettableTagged
				err := Unmarshal(strings.NewReader(`{"n":1}`), &v)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "not settable")
			},
		},
		{
			name: "stringFieldGotNumber",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					S string `jsonr:"s"`
				}
				err := Unmarshal(strings.NewReader(`{"s":1}`), &v)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "string")
			},
		},
		{
			name: "intFieldBadQuotedString",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					N int `jsonr:"n"`
				}
				err := Unmarshal(strings.NewReader(`{"n":"notint"}`), &v)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "integer")
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestUnmarshal_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "topLevelNullStruct",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					X int `jsonr:"x"`
				}
				require.NoError(t, Unmarshal(strings.NewReader(`null`), &v))
				assert.Equal(t, 0, v.X)
			},
		},
		{
			name: "numericIndexSegment",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					ID int `jsonr:"items.0.id"`
				}
				require.NoError(t, Unmarshal(strings.NewReader(`{"items":[{"id":42}]}`), &v))
				assert.Equal(t, 42, v.ID)
			},
		},
		{
			name: "wildcardEmptyArray",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					Names []string `jsonr:"people.*.name"`
				}
				require.NoError(t, Unmarshal(strings.NewReader(`{"people":[]}`), &v))
				assert.Empty(t, v.Names)
			},
		},
		{
			name: "pointerIntNull",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					P *int `jsonr:"p"`
				}
				require.NoError(t, Unmarshal(strings.NewReader(`{"p":null}`), &v))
				assert.Nil(t, v.P)
			},
		},
		{
			name: "pointerIntValue",
			run: func(t *testing.T) {
				t.Helper()
				var v struct {
					P *int `jsonr:"p"`
				}
				require.NoError(t, Unmarshal(strings.NewReader(`{"p":42}`), &v))
				require.NotNil(t, v.P)
				assert.Equal(t, 42, *v.P)
			},
		},
		{
			name: "topLevelSliceNull",
			run: func(t *testing.T) {
				t.Helper()
				type sub struct {
					A int `jsonr:"object.a"`
				}
				v := []sub{{A: 99}}
				require.NoError(t, Unmarshal(strings.NewReader(`null`), &v))
				assert.Nil(t, v)
			},
		},
		{
			name: "topLevelFixedArraySkipsExtraElements",
			run: func(t *testing.T) {
				t.Helper()
				type sub struct {
					A int `jsonr:"object.a"`
				}
				var v [1]sub
				in := `[{ "object": { "a": 1 } },{ "object": { "a": 2 } }]`
				require.NoError(t, Unmarshal(strings.NewReader(in), &v))
				assert.Equal(t, 1, v[0].A)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestInlineDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "nestedScalar",
			run: func(t *testing.T) {
				t.Helper()
				const in = `{"name": "North","address": { "coordinates": { "latitude": 12.5, "longitude": -3.01 } } }`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				lat, err := InlineDecode[float64](dec, "address.coordinates.latitude")
				require.NoError(t, err)
				assert.InDelta(t, 12.5, lat, 0)
				lng, err := InlineDecode[float64](jsontext.NewDecoder(strings.NewReader(in)), "address.coordinates.longitude")
				require.NoError(t, err)
				assert.InDelta(t, -3.01, lng, 0)
			},
		},
		{
			name: "arrayIndexPath",
			run: func(t *testing.T) {
				t.Helper()
				const in = `{"items":[{"id":42},{"id":99}]}`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				id, err := InlineDecode[int](dec, "items.0.id")
				require.NoError(t, err)
				assert.Equal(t, 42, id)
			},
		},
		{
			name: "rootArray",
			run: func(t *testing.T) {
				t.Helper()
				const in = `[{"k":1},{"k":2}]`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				v, err := InlineDecode[int](dec, "1.k")
				require.NoError(t, err)
				assert.Equal(t, 2, v)
			},
		},
		{
			name: "inlineDoc_nestedInt",
			run: func(t *testing.T) {
				t.Helper()
				const in = `{"x":{"y":{"z":1}}}`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				z, err := InlineDecode[int](dec, "x.y.z")
				require.NoError(t, err)
				assert.Equal(t, 1, z)
			},
		},
		{
			name: "inlineDoc_wildcardSlice",
			run: func(t *testing.T) {
				t.Helper()
				const in = `{"x":{"y":[{"z":1},{"z":2},{"z":3}]}}`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				z, err := InlineDecode[[]int](dec, "x.y.*.z")
				require.NoError(t, err)
				assert.Equal(t, []int{1, 2, 3}, z)
			},
		},
		{
			name: "inlineDoc_fullObjectMap",
			run: func(t *testing.T) {
				t.Helper()
				const in = `{"x":{"y":{"a":1,"b":2,"c":3}}}`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				z, err := InlineDecode[map[string]int](dec, "x.y")
				require.NoError(t, err)
				assert.Equal(t, map[string]int{"a": 1, "b": 2, "c": 3}, z)
			},
		},
		{
			name: "inlineDoc_multiFieldMap",
			run: func(t *testing.T) {
				t.Helper()
				const in = `{"x":{"y":{"a":1,"b":2,"c":3}}}`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				z, err := InlineDecode[map[string]int](dec, "x.y.[a,b]")
				require.NoError(t, err)
				assert.Equal(t, map[string]int{"a": 1, "b": 2}, z)
			},
		},
		{
			name: "inlineDoc_wildcardMultiFieldSliceMaps",
			run: func(t *testing.T) {
				t.Helper()
				const in = `{"x":{"y":[{"a":1,"b":2,"c":3},{"a":4,"b":5,"c":6},{"a":7,"b":8,"c":9}]}}`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				z, err := InlineDecode[[]map[string]int](dec, "x.y.*.[a,b]")
				require.NoError(t, err)
				assert.Equal(t, []map[string]int{
					{"a": 1, "b": 2},
					{"a": 4, "b": 5},
					{"a": 7, "b": 8},
				}, z)
			},
		},
		{
			name: "wildcardSkipsElementMissingPath",
			run: func(t *testing.T) {
				t.Helper()
				const in = `{"x":{"y":[{"z":1},{},{"z":3}]}}`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				z, err := InlineDecode[[]int](dec, "x.y.*.z")
				require.NoError(t, err)
				assert.Equal(t, []int{1, 3}, z)
			},
		},
		{
			name: "sliceValue",
			run: func(t *testing.T) {
				t.Helper()
				const in = `{"nums":[1,2,3]}`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				s, err := InlineDecode[[]int](dec, "nums")
				require.NoError(t, err)
				assert.Equal(t, []int{1, 2, 3}, s)
			},
		},
		{
			name: "skipsLargeSiblingSubtree",
			run: func(t *testing.T) {
				t.Helper()
				in := `{"big":[` + strings.Repeat(`{},`, 200) + `{}], "x": 7}`
				dec := jsontext.NewDecoder(strings.NewReader(in))
				x, err := InlineDecode[int](dec, "x")
				require.NoError(t, err)
				assert.Equal(t, 7, x)
			},
		},
		{
			name: "pointerNull",
			run: func(t *testing.T) {
				t.Helper()
				dec := jsontext.NewDecoder(strings.NewReader(`{"p":null}`))
				p, err := InlineDecode[*int](dec, "p")
				require.NoError(t, err)
				assert.Nil(t, p)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

func TestInlineDecode_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		runR func(t *testing.T) error
		want string
	}{
		{
			name: "keyNotFound",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[int](jsontext.NewDecoder(strings.NewReader(`{"a":1}`)), "missing")
				return err
			},
			want: `key "missing" not found`,
		},
		{
			name: "arrayIndexOutOfRange",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[int](jsontext.NewDecoder(strings.NewReader(`{"items":[]}`)), "items.0.id")
				return err
			},
			want: "out of range",
		},
		{
			name: "nonNumericArraySegment",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[int](jsontext.NewDecoder(strings.NewReader(`[]`)), "notint")
				return err
			},
			want: "not a valid array index",
		},
		{
			name: "scalarRootWithFurtherPath",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[int](jsontext.NewDecoder(strings.NewReader(`42`)), "x")
				return err
			},
			want: "cannot traverse",
		},
		{
			name: "rootString",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[int](jsontext.NewDecoder(strings.NewReader(`"hi"`)), "x")
				return err
			},
			want: "cannot traverse",
		},
		{
			name: "multiFieldKeyMissing",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[map[string]int](jsontext.NewDecoder(strings.NewReader(`{}`)), "y.[a,b]")
				return err
			},
			want: `key "y" not found`,
		},
		{
			name: "multiFieldNotLast",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[map[string]int](jsontext.NewDecoder(strings.NewReader(`{}`)), "y.[a,b].z")
				return err
			},
			want: "multi-field segment must be last",
		},
		{
			name: "wildcardRequiresSliceDestination",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[string](jsontext.NewDecoder(strings.NewReader(`{}`)), "a.*.b")
				return err
			},
			want: "wildcard path requires slice",
		},
		{
			name: "structDestinationRejected",
			runR: func(t *testing.T) error {
				t.Helper()
				type coord struct {
					Lat float64
				}
				_, err := InlineDecode[coord](jsontext.NewDecoder(strings.NewReader(`{"c":{"Lat":1}}`)), "c")
				return err
			},
			want: "cannot be struct type",
		},
		{
			name: "multiFieldRequiresMapDestination",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[[]int](jsontext.NewDecoder(strings.NewReader(`{"x":{"y":1}}`)), "x.y.[a,b]")
				return err
			},
			want: "multi-field path requires map",
		},
		{
			name: "emptyPath",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[int](jsontext.NewDecoder(strings.NewReader(`{}`)), " ")
				return err
			},
			want: "empty jsonr path",
		},
		{
			name: "typeMismatch",
			runR: func(t *testing.T) error {
				t.Helper()
				_, err := InlineDecode[string](jsontext.NewDecoder(strings.NewReader(`{"s":1}`)), "s")
				return err
			},
			want: "string",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.runR(t)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
