package store_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRootModuleGraphExcludesGorm(t *testing.T) {
	t.Parallel()

	command := exec.Command("go", "list", "-m", "all")
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list -m all error = %v", err)
	}

	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 0 && strings.HasPrefix(fields[0], "gorm.io/") {
			t.Fatalf("module graph contains ORM dependency %q", fields[0])
		}
	}
}

func TestRootPackagesExcludeConfigurationParsers(t *testing.T) {
	t.Parallel()

	command := exec.Command("go", "list", "-deps", "./...")
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list -deps ./... error = %v", err)
	}

	parserPackages := []string{
		"github.com/pelletier/go-toml/v2",
		"gopkg.in/yaml.v3",
	}
	for _, packagePath := range strings.Fields(string(output)) {
		for _, parserPackage := range parserPackages {
			if packagePath == parserPackage || strings.HasPrefix(packagePath, parserPackage+"/") {
				t.Fatalf("package graph contains configuration parser %q", packagePath)
			}
		}
	}
}
