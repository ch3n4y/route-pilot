//go:build windows

package routecmd

import "testing"

func TestDeleteFromStoreRejectsInvalidStore(t *testing.T) {
	if err := DeleteFromStore("10.0.0.0/8", "192.168.1.2", "unknown"); err == nil {
		t.Fatal("expected invalid store error")
	}
}
