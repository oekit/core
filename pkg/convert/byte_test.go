package convert

import (
	"testing"
)

func TestBytesToString(t *testing.T) {
	result := BytesToString([]byte("hello"))
	if result != "hello" {
		t.Errorf("BytesToString() = %v, want hello", result)
	}
}

func TestStringToBytes(t *testing.T) {
	result := StringToBytes("hello")
	if string(result) != "hello" {
		t.Errorf("StringToBytes() = %v, want hello", string(result))
	}
}

func TestBytesToInt(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want int
	}{
		{"valid", []byte("123"), 123},
		{"invalid", []byte("abc"), 0},
		{"empty", []byte(""), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BytesToInt(tt.data); got != tt.want {
				t.Errorf("BytesToInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBytesToInt64(t *testing.T) {
	result := BytesToInt64([]byte("123"))
	if result != 123 {
		t.Errorf("BytesToInt64() = %v, want 123", result)
	}
}

func TestBytesToFloat32(t *testing.T) {
	result := BytesToFloat32([]byte("3.14"))
	if result < 3.13 || result > 3.15 {
		t.Errorf("BytesToFloat32() = %v, want ~3.14", result)
	}
}

func TestBytesToFloat64(t *testing.T) {
	result := BytesToFloat64([]byte("3.14"))
	if result < 3.13 || result > 3.15 {
		t.Errorf("BytesToFloat64() = %v, want ~3.14", result)
	}
}

func TestByteToBytes(t *testing.T) {
	result := ByteToBytes('a')
	if len(result) != 1 || result[0] != 'a' {
		t.Errorf("ByteToBytes() = %v, want [97]", result)
	}
}
