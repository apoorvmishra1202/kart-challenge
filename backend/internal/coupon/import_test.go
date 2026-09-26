package coupon

import "testing"

func TestParseIndexOptions(t *testing.T) {
	tests := []struct {
		name         string
		mem, workers string
		want         IndexOptions
		wantErr      bool
	}{
		{"defaults", "", "", IndexOptions{"1GB", 4}, false},
		{"explicit", "512MB", "8", IndexOptions{"512MB", 8}, false},
		{"kB and zero workers", "65536kB", "0", IndexOptions{"65536kB", 0}, false},
		{"max workers", "2GB", "16", IndexOptions{"2GB", 16}, false},
		{"too many workers", "1GB", "17", IndexOptions{}, true},
		{"negative workers", "1GB", "-1", IndexOptions{}, true},
		{"non-integer workers", "1GB", "4; DROP TABLE x", IndexOptions{}, true},
		{"no unit", "1024", "", IndexOptions{}, true},
		{"lowercase unit", "1gb", "", IndexOptions{}, true},
		{"injection", "1GB'; DROP TABLE coupon_codes; --", "", IndexOptions{}, true},
		{"space", "1 GB", "", IndexOptions{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIndexOptions(tt.mem, tt.workers)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
