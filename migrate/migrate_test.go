package migrate

import (
	"reflect"
	"testing"
)

func TestParseFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     *Migration
		wantErr  bool
	}{
		// Valid versioned
		{
			filename: "V001__CreateUsersTable.sql",
			want: &Migration{
				Version:     "001",
				Description: "CreateUsersTable",
				Type:        "SQL",
				Script:      "V001__CreateUsersTable.sql",
			},
			wantErr: false,
		},
		{
			filename: "V999__AddIndexes.sql",
			want: &Migration{
				Version:     "999",
				Description: "AddIndexes",
				Type:        "SQL",
				Script:      "V999__AddIndexes.sql",
			},
			wantErr: false,
		},
		// Valid repeatable
		{
			filename: "R__CreateViews.sql",
			want: &Migration{
				Version:     "",
				Description: "CreateViews",
				Type:        "REPEATABLE",
				Script:      "R__CreateViews.sql",
			},
			wantErr: false,
		},
		{
			filename: "R__seed_reference_data.sql",
			want: &Migration{
				Version:     "",
				Description: "seed_reference_data",
				Type:        "REPEATABLE",
				Script:      "R__seed_reference_data.sql",
			},
			wantErr: false,
		},
		// Invalid cases
		{
			filename: "V001_CreateUsersTable.sql", // Missing double underscore
			want:     nil,
			wantErr:  true,
		},
		{
			filename: "Vabc__CreateUsersTable.sql", // Non-numeric version
			want:     nil,
			wantErr:  true,
		},
		{
			filename: "V001__createUsersTable.sql", // Description not PascalCase
			want:     nil,
			wantErr:  true,
		},
		{
			filename: "V001__CreateUsersTable.txt", // Wrong extension
			want:     nil,
			wantErr:  true,
		},
		{
			filename: "R_CreateViews.sql", // Missing double underscore for repeatable
			want:     nil,
			wantErr:  true,
		},
		{
			filename: "random_file.sql", // Doesn't start with V or R__
			want:     nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got, err := ParseFilename(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFilename() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFilename() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateChecksum(t *testing.T) {
	content1 := []byte("SELECT * FROM users;\r\nSELECT * FROM profiles;\r\n")
	content2 := []byte("SELECT * FROM users;\nSELECT * FROM profiles;\n")

	checksum1 := CalculateChecksum(content1)
	checksum2 := CalculateChecksum(content2)

	if checksum1 != checksum2 {
		t.Errorf("CalculateChecksum() did not normalize line endings: %d != %d", checksum1, checksum2)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want int // -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
	}{
		{"001", "002", -1},
		{"002", "001", 1},
		{"001", "1", 0},
		{"2", "10", -1},
		{"10", "2", 1},
		{"9223372036854775807", "9223372036854775808", -1}, // Big integer comparison test
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%s, %s) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}
