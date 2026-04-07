package jsonr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeTime(t *testing.T) {
	t.Parallel()

	locPlus2 := time.FixedZone("plus2", 2*60*60)

	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr string
	}{
		{
			name:  "RFC3339Z",
			input: "2024-03-15T14:30:00Z",
			want:  time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:  "RFC3339NanoZ",
			input: "2024-03-15T14:30:00.123456789Z",
			want:  time.Date(2024, 3, 15, 14, 30, 0, 123456789, time.UTC),
		},
		{
			name:  "RFC3339Offset",
			input: "2024-03-15T12:30:00+02:00",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "RFC3339Offset_roundTripSameWallInZone",
			input: "2024-03-15T14:30:00+02:00",
			want:  time.Date(2024, 3, 15, 14, 30, 0, 0, locPlus2).UTC(),
		},
		{
			name:  "dateOnly",
			input: "2024-06-01",
			want:  time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "datetimeNoOffsetT",
			input: "2024-06-01T13:45:30",
			want:  time.Date(2024, 6, 1, 13, 45, 30, 0, time.UTC),
		},
		{
			name:  "datetimeNoOffsetSpace",
			input: "2024-06-01 13:45:30",
			want:  time.Date(2024, 6, 1, 13, 45, 30, 0, time.UTC),
		},
		{
			name:  "timeOnlyHMS",
			input: "13:45:30",
			want:  time.Date(0, 1, 1, 13, 45, 30, 0, time.UTC),
		},
		{
			name:  "timeOnlyHM",
			input: "13:45",
			want:  time.Date(0, 1, 1, 13, 45, 0, 0, time.UTC),
		},
		{
			name:  "trimSpace",
			input: "  2024-01-02T00:00:00Z  ",
			want:  time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "empty",
			input:   "",
			wantErr: "empty time string",
		},
		{
			name:    "whitespaceOnly",
			input:   "   ",
			wantErr: "empty time string",
		},
		{
			name:    "invalid",
			input:   "not-a-time",
			wantErr: "cannot parse time string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := decodeTime(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, tt.want.Equal(got), "got %v want %v", got, tt.want)
		})
	}
}
