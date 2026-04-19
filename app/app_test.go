package app

import (
	"testing"
	"time"
)

func TestParseSkipDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		year    int
		want    time.Time
		wantErr bool
	}{
		{
			name:  "new format: day month",
			input: "25 April",
			year:  2026,
			want:  time.Date(2026, time.April, 25, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "new format: single digit day",
			input: "5 April",
			year:  2026,
			want:  time.Date(2026, time.April, 5, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "old format: day-of-week day month",
			input: "Saturday 31 January",
			year:  2026,
			want:  time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "old format: zero-padded day",
			input: "Saturday 05 April",
			year:  2026,
			want:  time.Date(2026, time.April, 5, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "invalid: random text",
			input:   "Dates and locations",
			year:    2026,
			wantErr: true,
		},
		{
			name:    "invalid: empty string",
			input:   "",
			year:    2026,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSkipDate(tt.input, tt.year)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSkipDate(%q, %d) error = %v, wantErr %v", tt.input, tt.year, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("parseSkipDate(%q, %d) = %v, want %v", tt.input, tt.year, got, tt.want)
			}
		})
	}
}

func TestParseLocationLine(t *testing.T) {
	date := time.Date(2026, time.April, 25, 0, 0, 0, 0, time.UTC)
	dateStr := "25 April"

	tests := []struct {
		name         string
		input        string
		wantAddress  string
		wantPostcode string
	}{
		{
			name:         "plain format",
			input:        "Larch Close, SW12 9SY",
			wantAddress:  "Larch Close",
			wantPostcode: "SW12 9SY",
		},
		{
			name:         "numbered prefix",
			input:        "1.  Larch Close, SW12 9SY",
			wantAddress:  "Larch Close",
			wantPostcode: "SW12 9SY",
		},
		{
			name:         "multi-part address",
			input:        "Lindsay Court, Battersea High Street, SW11 3HZ",
			wantAddress:  "Lindsay Court",
			wantPostcode: "SW11 3HZ",
		},
		{
			name:         "numbered prefix with multi-part address",
			input:        "5.  Lindsay Court, Battersea High Street, SW11 3HZ",
			wantAddress:  "Lindsay Court",
			wantPostcode: "SW11 3HZ",
		},
		{
			name:         "longer description",
			input:        "Fitzhugh Estate car park, in front of Gernigan House, SW18 3SG",
			wantAddress:  "Fitzhugh Estate car park",
			wantPostcode: "SW18 3SG",
		},
		{
			name:        "empty string",
			input:       "",
			wantAddress: "",
		},
		{
			name:        "no comma",
			input:       "Some random text",
			wantAddress: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLocationLine(tt.input, date, dateStr)
			if got.Address != tt.wantAddress {
				t.Errorf("parseLocationLine(%q).Address = %q, want %q", tt.input, got.Address, tt.wantAddress)
			}
			if tt.wantPostcode != "" && got.Postcode != tt.wantPostcode {
				t.Errorf("parseLocationLine(%q).Postcode = %q, want %q", tt.input, got.Postcode, tt.wantPostcode)
			}
		})
	}
}
