package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ryanuber/go-license"
)

// /home/dgodd/go/src/github.com/buildpack/pack

func goModDirs() (map[string]string, error) {
	cmd := exec.Command("go", "mod", "graph")
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	txt, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Join(os.Getenv("HOME"), "go/pkg/mod")
	out := make(map[string]string)
	for _, line := range strings.Split(string(txt), "\n") {
		a := strings.SplitN(line, " ", 3)
		if line == "" {
			// no-op
		} else if len(a) != 2 {
			fmt.Printf("BAD LINE (%d): %s\n", len(a), line)
		} else {
			out[a[1]] = filepath.Join(baseDir, a[1])
		}
	}
	return out, nil
}

func licenseFromGithub(name string) (string, error) {
	if !strings.HasPrefix(name, "github.com/") {
		return "", fmt.Errorf("not github: %s", name)
	}
	name = strings.Split(name[11:], "@")[0]
	url := "https://api.github.com/repos/" + name
	// fmt.Println("curl", "-i", "--silent", url, "| head -30")
	res, err := http.Get(url + "?access_token=" + os.Getenv("GITHUB_ACCESS_TOKEN"))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("expected OK: %d", res.StatusCode)
	}

	var out struct {
		License struct {
			SpdxID string `json:"spdx_id"`
			Name   string `json:"name"`
		} `json:"license"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}

	if out.License.SpdxID != "" {
		return out.License.SpdxID, nil
	}
	if out.License.Name != "" {
		return out.License.Name, nil
	}
	if out.Message != "" {
		return "", errors.New(out.Message)
	}

	return "", errors.New("Unkown")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func printTable(m map[string]string) {
	maxNameLen := 0
	for name, _ := range m {
		name = strings.Split(name, "@")[0]
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}
	nameFormat := fmt.Sprintf("%%-%ds %%s\n", maxNameLen)
	for _, name := range sortedKeys(m) {
		fmt.Printf(nameFormat, strings.Split(name, "@")[0], m[name])
	}
}

func main() {
	mods, err := goModDirs()
	if err != nil {
		panic(err)
	}

	licenses := make(map[string]string)
	notfound := make(map[string]string)

	for name, dir := range mods {
		l, err := license.NewFromDir(dir)
		if err != nil {
			if ghl, err2 := licenseFromGithub(name); err2 == nil {
				licenses[name] = ghl + " (GH)"
			} else if os.IsNotExist(err) {
				notfound[name] = "NOT FOUND"
			} else {
				notfound[name] = err.Error()
			}
		} else {
			licenses[name] = l.Type
		}
	}

	printTable(licenses)
	if len(notfound) > 0 {
		fmt.Println("")
		printTable(notfound)
	}
}
