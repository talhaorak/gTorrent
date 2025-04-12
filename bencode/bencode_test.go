package bencode

import (
	"reflect"
	"testing"
)

func TestDecode(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    *Data
		wantErr error
	}{
		{
			name:    "Empty content",
			content: []byte{},
			want:    nil,
			wantErr: nil,
		},
		{
			name:    "Byte string",
			content: []byte("4:spam"),
			want:    NewData("spam"),
			wantErr: nil,
		},
		{
			name:    "Integer",
			content: []byte("i42e"),
			want:    NewData(42),
			wantErr: nil,
		},
		{
			name:    "Negative Integer",
			content: []byte("i-42e"),
			want:    NewData(-42),
			wantErr: nil,
		},
		{
			name:    "List",
			content: []byte("l4:spam4:eggse"),
			want:    NewData([]any{"spam", "eggs"}),
			wantErr: nil,
		}, {
			name:    "List within List",
			content: []byte("l4:spaml1:a1:bee"),
			want:    NewData([]any{"spam", []any{"a", "b"}}),
			wantErr: nil,
		},
		{
			name:    "Dictionary",
			content: []byte("d3:cow3:moo4:spam4:eggs3:numi42ee"),
			want:    NewData(map[string]any{"cow": "moo", "spam": "eggs", "num": 42}),
			wantErr: nil,
		},
		// Add more test cases here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := Decode(tt.content)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decode() got = %s, want %s", got.String(), tt.want.String())
			}
			if err != tt.wantErr {
				t.Errorf("Decode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
func TestEncode(t *testing.T) {
	tests := []struct {
		name string
		data Data
		want []byte
	}{
		{
			name: "String",
			data: *NewData("spam"),
			want: []byte("4:spam"),
		},
		{
			name: "Integer",
			data: *NewData(42),
			want: []byte("i42e"),
		},
		{
			name: "List",
			data: *NewData([]*Data{
				NewData("spam"),
				NewData("eggs"),
			}),
			want: []byte("l4:spam4:eggse"),
		},
		{
			name: "Dictionary",
			data: *NewData(map[string]*Data{
				"cow":  NewData("moo"),
				"spam": NewData("eggs"),
			}),
			want: []byte("d3:cow3:moo4:spam4:eggse"),
		},
		// Add more test cases here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Encode(&tt.data)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Encode() got = %s, want %s", string(got), string(tt.want))
			}
		})
	}
}
