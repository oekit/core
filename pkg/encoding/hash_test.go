package encoding

import (
	"testing"
)

func TestSha256Hex(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"empty", []byte(""), "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"hello", []byte("hello"), "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sha256Hex(tt.data); got != tt.want {
				t.Errorf("Sha256Hex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSha256(t *testing.T) {
	result := Sha256("hello")
	if result != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Errorf("Sha256() = %v, want %v", result, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
	}
}

func TestBase64Encode(t *testing.T) {
	result := Base64Encode("hello")
	if result != "aGVsbG8=" {
		t.Errorf("Base64Encode() = %v, want %v", result, "aGVsbG8=")
	}
}

func TestBase64Decode(t *testing.T) {
	result, err := Base64Decode("aGVsbG8=")
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello" {
		t.Errorf("Base64Decode() = %v, want %v", result, "hello")
	}
}

func TestBase64URLEncode(t *testing.T) {
	result := Base64URLEncode("hello")
	if result != "aGVsbG8=" {
		t.Errorf("Base64URLEncode() = %v, want %v", result, "aGVsbG8=")
	}
}

func TestBase64URLDecode(t *testing.T) {
	result, err := Base64URLDecode("aGVsbG8=")
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello" {
		t.Errorf("Base64URLDecode() = %v, want %v", result, "hello")
	}
}

func TestFnv1a64(t *testing.T) {
	result := Fnv1a64([]byte("hello"))
	if result == 0 {
		t.Error("Fnv1a64() should not return 0")
	}
	// FNV-1a 64 位标准向量："hello" 的已知值，锁定算法不被无意更改
	if result != 0xa430d84680aabd0b {
		t.Errorf("Fnv1a64(hello) = %#x, want %#x", result, uint64(0xa430d84680aabd0b))
	}
}
