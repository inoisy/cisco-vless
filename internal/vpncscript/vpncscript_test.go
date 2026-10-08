package vpncscript

import (
	"reflect"
	"testing"
)

func TestSafeDomains(t *testing.T) {
	got := SafeDomains([]string{"Corp.RU", "corp.ru", "", ".evil", "a/../b", "x y", "my-host.local", "a;rm -rf"})
	want := []string{"corp.ru", "my-host.local"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
