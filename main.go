package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ryanuber/go-license"
)

func directMods(path string) (map[string]struct{}, error) {
	mods := make(map[string]struct{})
	txt, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	inRequire := false
	for _, line := range strings.Split(string(txt), "\n") {
		line = strings.TrimSpace(line)
		if line == "require (" {
			inRequire = true
			continue
		}
		if line == ")" {
			inRequire = false
			continue
		}
		if inRequire {
			a := strings.Split(line, " ")
			if len(a) == 2 {
				mods[a[0]] = struct{}{}
			}
		}
	}
	return mods, nil
}

func allModDirs(path string) (map[string]string, error) {
	cmd := exec.Command("go", "mod", "graph")
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	cmd.Dir = path
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
	// direct, err := directMods()
	// if err != nil {
	// 	panic(err)
	// }

	gomods, err := filepath.Glob("*/go.mod")
	if err != nil {
		panic(err)
	}
	allmods := make(map[string]string)
	for _, gomod := range gomods {
		mods, err := allModDirs(filepath.Dir(gomod))
		if err != nil {
			panic(err)
		}
		for k, v := range mods {
			allmods[k] = v
		}
	}

	licenses := make(map[string]string)
	notfound := make(map[string]string)

	for name, dir := range allmods {
		l, err := license.NewFromDir(dir)
		if err != nil {
			if ghl, err2 := licenseFromGithub(name); err2 == nil {
				licenses[name] = ghl
			} else if os.IsNotExist(err) {
				notfound[name] = "NOT FOUND"
			} else {
				notfound[name] = err.Error()
			}
		} else {
			licenses[name] = l.Type
		}
	}

	fmt.Println("# Direct gomod dependencies")
	for _, gomod := range gomods {
		fmt.Println("")
		fmt.Println("## ", filepath.Base(filepath.Dir(gomod)))
		direct, err := directMods(gomod)
		if err != nil {
			panic(err)
		}
		printed := make(map[string]bool)
		for _, name := range sortedKeys(licenses) {
			short := strings.Split(name, "@")[0]
			if !printed[short+licenses[name]] {
				if _, ok := direct[short]; ok {
					fmt.Printf("- [https://%s](%s) (%s)\n", short, short, licenses[name])
				}
			}
			printed[short+licenses[name]] = true
		}
	}

	fmt.Println("")
	fmt.Println("# Consolidated go mod graph dependencies")
	printed := make(map[string]bool)
	for _, name := range sortedKeys(licenses) {
		short := strings.Split(name, "@")[0]
		if !printed[short+licenses[name]] {
			fmt.Printf("- [%s](https://%s) (%s)\n", short, short, licenses[name])
		}
		printed[short+licenses[name]] = true
	}
}
