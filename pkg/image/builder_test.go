package image

import (
	"testing"
)

func TestNewBuilder(t *testing.T) {
    b := NewBuilder("token")
    if b == nil {
        t.Fatal("NewBuilder returned nil")
    }
}
