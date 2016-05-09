package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	verbose = true
)

func main() {
	project := os.Getenv("GB_PROJECT_DIR")

	//sanity
	if project == "" {
		panic("gb extensions should be launched with GB_PROJECT_DIR set")
	}
	//validate that gopherjs is there
	if err := validateGopherjsInPath(project); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	//figure out args, if any
	args := os.Args[1:]
	if len(args) == 0 {
		help()
		os.Exit(0)
	}

	//walk each arg, assuming that they are golang package specs
	for _, arg := range args {

		//validate that the packages provided have a client subpackage
		//and the static/en/web directory, as expected
		if err := validateClientPackage(project, arg); err != nil {
			fmt.Fprintf(os.Stderr, "Unable to find client package in %s\n",
				constructClientPackagePath(project, arg))
			os.Exit(1)
		}
		if err := validateStaticEnglishDir(project, arg); err != nil {
			fmt.Fprintf(os.Stderr, "Unable to find static/en/web directory, expected it to be %s\n",
				constructStaticEnglishPath(project, arg))
			os.Exit(1)
		}
		//this the full path to the package from arg
		dir := constructClientPackagePath(project, arg)

		//find the gofiles in the package
		gofiles, err := iterateDirs([]string{dir})
		if err != nil {
			os.Exit(1)
		}

		//find the gofiles that have a main()
		pages := []string{}
		for _, gofile := range gofiles {
			hasMain, err := hasMainFunc(gofile)
			if err != nil {
				os.Exit(1)
			}
			if hasMain {
				pages = append(pages, gofile)
			}
		}

		//walk each page, compiling to the static/en/web
		for _, page := range pages {
			if !strings.HasPrefix(page, constructClientPackagePath(project, arg)) {
				panic(fmt.Sprintf("unable to understand page path %s in package %s",
					page, constructClientPackagePath(project, arg)))
			}
			suffix := strings.TrimPrefix(page, constructClientPackagePath(project, arg))
			target := filepath.Join(constructStaticEnglishPath(project, arg), suffix)
			if err := launchGopherjs(project, "build", "-m", "-o", target, page); err != nil {
				os.Exit(1)
			}
		}
	}
}

func iterateDirs(dirs []string) ([]string, error) {
	gofiles := []string{}
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Fprintf(os.Stderr, "error walking %s: %v\n", path, err)
				return err
			}
			if strings.HasSuffix(info.Name(), ".go") {
				gofiles = append(gofiles, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return gofiles, nil
}

func hasMainFunc(path string) (bool, error) {
	fset := token.NewFileSet() // positions are relative to fset

	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing %s: %v", path, err)
		return false, err
	}

	for _, decl := range f.Decls {
		switch x := decl.(type) {
		case *ast.FuncDecl:
			if x.Name.String() == "main" {
				return true, nil
			}
		}
	}
	return false, nil
}

func constructClientPackagePath(project string, arg string) string {
	return filepath.Join(project, "src", arg, "client")
}

func constructStaticEnglishPath(project string, arg string) string {
	return filepath.Join(project, "src", arg, "static", "en", "web")
}

func validateClientPackage(projectDir string, arg string) error {
	path := constructClientPackagePath(projectDir, arg)
	_, err := os.Stat(path)
	return err
}

func validateStaticEnglishDir(projectDir string, arg string) error {
	path := constructStaticEnglishPath(projectDir, arg)
	_, err := os.Stat(path)
	return err
}

func validateGopherjsInPath(projectDir string) error {
	cmd := exec.Command("gopherjs")
	cmd.Env = append(os.Environ(), "GOPATH="+projectDir)
	return cmd.Run()
}

func launchGopherjs(projectDir string, args ...string) error {
	cmd := exec.Command("gopherjs", args...)
	vendor := projectDir + string(os.PathSeparator) + "vendor"
	bothDirs := projectDir + string(os.PathListSeparator) + vendor
	cmd.Env = append(os.Environ(), "GOPATH="+bothDirs)
	out, err := cmd.CombinedOutput()
	fmt.Printf("%s", string(out))
	return err
}

func help() {
	fmt.Printf("gb seven5 requires a package name to build client software from\n")
}
