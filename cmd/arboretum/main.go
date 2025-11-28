package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var aboutText = `This package is the entry point to the main microservice app.
Its task is to perform some preliminary system checks before calling kubernetes to start cluster initialization.
Since cluster initialization can take some time depending on the configuration, we would like to detect issues early.
`

var requiredDeps = []string{"minikube", "docker", "kubectl"}
var requiredDirs = []string{"cmd", "manifests", "services", "arboretum", "fetcher", "transformer", "web", "writer"}

func findRequiredDeps(deps []string) map[string]string {
	found := make(map[string]string)
	for _, dep := range deps {
		fpath, pathErr := exec.LookPath(dep)
		if pathErr != nil {
			log.Printf("Looks like '%s' is not installed because of %v\n", dep, pathErr)
		}
		found[dep] = fpath
	}
	return found
}

func manifestsAreValid() bool {
	var result any
	// the Go dir tranversal uses a callback pattern which is unusual compared to other languages.
	walkErr := filepath.WalkDir("manifests", func(path string, d fs.DirEntry, err error) error {
		// we should check if we have fs error passed to WalkDir early on and stop
		if err != nil {
			log.Printf("Error walking 'manifests' dir because of %v\n", err)
			return err
		}
		if !d.IsDir() {
			// the ReadFile does closing automatically, neat!
			file, readErr := os.ReadFile(path)
			if readErr != nil {
				log.Printf("Error reading file %s because of %v!\n", file, readErr)
				return readErr
			}
			yamlErr := yaml.Unmarshal(file, &result)
			if yamlErr != nil {
				log.Printf("YAML validation failed on %s because of %v!\n", path, yamlErr)
				return yamlErr
			}
		}
		return err
	})
	return walkErr == nil
}

func dirStructureIsValid(dirs []string) bool {
	var matches int
	walkErr := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("Error walking root dir because of %v\n", err)
			return err
		}

		if d.IsDir() && !strings.HasPrefix(path, ".") {
			splitPath := strings.Split(path, string(os.PathSeparator))
			childDir := splitPath[len(splitPath)-1]
			if slices.Contains(dirs, childDir) {
				matches++
			}
		}
		return err
	})
	if walkErr != nil {
		log.Printf("Error walking root dir because of %v\n", walkErr)
	}
	return matches >= len(dirs)
}

func stopCluster() error {
	log.Printf("Stopping cluster\n")
	cmd := exec.Command("minikube", "stop", "-p", "arboretum")
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to stop the cluster because of %v\n", err)
		return err
	}
	return nil
}

// TODO: Let's keep it fixed here for now, later use conf files
func startCluster() error {
	if err := stopCluster(); err != nil {
		return err
	}
	cmd := exec.Command("minikube", "start", "--nodes", "1", "-p", "arboretum", "--insecure-registry=192.168.67.2:30000")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Running minikube caused %v because of %v\n", err, string(out))
		return err
	}
	log.Printf("%v\n", string(out))

	// wait until the cluster is ready
	log.Printf("Waiting for cluster initialization\n")
	for i := 0; i < 30; i++ {
		cmd = exec.Command("kubectl", "cluster-info")
		if err := cmd.Run(); err == nil {
			break
		}
		if i == 30 {
			log.Fatal("Cluster failed to initialize")
			return err
		}
		time.Sleep(2 * time.Second)
	}

	// apply manifests
	cmd = exec.Command("kubectl", "apply", "-f", "manifests/basicpod-with-port.yaml")
	out, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("Running kubectl caused %v because of %v\n", err, string(out))
		return err
	}
	log.Printf("%v\n", string(out))
	return nil
}

func main() {
	needAbout := flag.Bool("help", false, aboutText)
	checkDeps := flag.Bool("checkDeps", false, "check if system has required dependencies")
	checkDirs := flag.Bool("checkDirs", false, "check if project dir structure is valid")
	checkManifests := flag.Bool("checkManifests", false, "check if manifests can be parsed")

	flag.Parse()

	if *needAbout {
		flag.Usage()
		return
	}

	if *checkDeps {
		found := findRequiredDeps(requiredDeps)
		if len(found) != len(requiredDeps) {
			fmt.Printf("Error, found dependencies %#v but required %#v\n", found, requiredDeps)
			return
		}
		fmt.Println("All dependencies installed")
	}

	if *checkDirs {
		if !dirStructureIsValid(requiredDirs) {
			fmt.Printf("Error, dir structure is invalid. Expected dirs %#v\n", requiredDirs)
			return
		}
		fmt.Println("Dir structure is consistent")
	}

	if *checkManifests {
		if !manifestsAreValid() {
			fmt.Printf("Error, manifests are not valid!\n")
			return
		}
		fmt.Println("Manifests are valid YAML files")
	}

	if err := startCluster(); err != nil {
		fmt.Println("Oops")
	}
}
