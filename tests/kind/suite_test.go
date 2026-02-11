//go:build kind

package kind

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

var fw *Framework

func TestMain(m *testing.M) {
	fw = NewFramework()

	if err := fw.Setup(); err != nil {
		// Skip gracefully for common issues
		if strings.Contains(err.Error(), "kind not found") {
			fmt.Println("SKIP: kind not installed")
			os.Exit(0)
		}
		if strings.Contains(err.Error(), "docker not running") {
			fmt.Println("SKIP: docker not running")
			os.Exit(0)
		}
		if strings.Contains(err.Error(), "kubectl not found") {
			fmt.Println("SKIP: kubectl not found")
			os.Exit(0)
		}
		fmt.Printf("Setup failed: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	fw.Teardown()
	os.Exit(code)
}
